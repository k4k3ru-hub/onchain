"""Evaluate discovered LP exits under the observed configuration.

This experiment combines source-derived direct-exit analysis with a documented
manual review of the remaining call graph. It is not an arbitrary-contract
security proof or a production decision engine. No custodian names, selectors,
addresses, storage slots, record IDs, dates or reference results are predefined.
"""

import argparse
import hashlib
import json
from fractions import Fraction
from pathlib import Path

from analyze_source import Analyzer, ASSET_OPERATIONS, walk


UNKNOWN = object()


class CurrentState(Analyzer):
    def __init__(self, compiled, source, discovery, observed):
        super().__init__(compiled, source)
        self.discovery = discovery
        self.observed = observed
        self.plan = self.build()
        if len(self.plan["records"]) != 1:
            raise ValueError("record_shape_unsupported")
        shape = self.plan["records"][0]
        self.record_values = {}
        if len(shape["time_fields"]) != len(observed["time_fields"]) or len(self.plan["mutable_gates"]) != len(observed["mutable_gates"]):
            raise ValueError("observation_shape_mismatch")
        for field, value in zip(shape["time_fields"], observed["time_fields"]):
            self.record_values[field["field_ast_id"]] = int(value["value"])
        self.record_values[shape["manager_field"]["field_ast_id"]] = int(discovery["manager"], 16)
        self.record_values[shape["nft_field"]["field_ast_id"]] = int(discovery["token_id"])
        self.states = {}
        for field, value in zip(self.plan["mutable_gates"], observed["mutable_gates"]):
            matches = [k for k, v in self.variables.items() if v["slot"] == field["slot"] and v["offset"] == field["offset"]]
            if len(matches) != 1:
                raise ValueError("gate_storage_ambiguous")
            self.states[matches[0]] = int(value["address"], 16)
        self.scope = None

    def scalar(self, expression, aliases):
        if expression.get("nodeType") == "BinaryOperation":
            left = self.scalar(expression["leftExpression"], aliases)
            right = self.scalar(expression["rightExpression"], aliases)
            if left is UNKNOWN or right is UNKNOWN:
                return UNKNOWN
            op = expression["operator"]
            operations = {"<": lambda: left < right, "<=": lambda: left <= right,
                          ">": lambda: left > right, ">=": lambda: left >= right,
                          "==": lambda: left == right, "!=": lambda: left != right,
                          "&&": lambda: bool(left and right), "||": lambda: bool(left or right)}
            return operations[op]() if op in operations else UNKNOWN
        if expression.get("nodeType") == "UnaryOperation" and expression.get("operator") == "!":
            value = self.scalar(expression["subExpression"], aliases)
            return UNKNOWN if value is UNKNOWN else not value
        value = self.value(expression, aliases)
        if not value:
            return UNKNOWN
        if value["kind"] == "literal":
            return value["value"]
        if value["kind"] == "timestamp":
            return self.discovery["block_timestamp"]
        if value["kind"] == "state":
            return self.states.get(value["variable"], UNKNOWN)
        if value["kind"] == "record_field" and (value["variable"], value["index"]) == self.scope:
            return self.record_values.get(value["field"], UNKNOWN)
        return UNKNOWN

    def paths(self, statement, target, aliases):
        """Track branches and dominating failed checks before one asset call."""
        if not statement:
            return {"continue"}
        kind = statement["nodeType"]
        if kind in ["Block", "UncheckedBlock"]:
            outcomes = {"continue"}
            for item in statement.get("statements", []):
                if "continue" not in outcomes:
                    break
                outcomes.remove("continue")
                outcomes.update(self.paths(item, target, aliases))
            return outcomes
        if kind == "IfStatement":
            # Calls in conditions must not be omitted from the effect analysis.
            if any(n.get("id") == target for n in walk(statement["condition"])):
                return {"reached"}
            condition = self.scalar(statement["condition"], aliases)
            if condition is UNKNOWN:
                return self.paths(statement["trueBody"], target, aliases) | self.paths(statement.get("falseBody"), target, aliases)
            return self.paths(statement["trueBody"] if condition else statement.get("falseBody"), target, aliases)
        if any(n.get("id") == target for n in walk(statement)):
            return {"reached"}
        if kind == "RevertStatement":
            for node in walk(statement):
                if node.get("nodeType") == "Literal" and node.get("kind") == "string":
                    self.reasons.add(node["value"])
            return {"blocked"}
        if kind == "Return":
            return {"blocked"}
        if kind in ["ForStatement", "WhileStatement", "DoWhileStatement", "TryStatement", "InlineAssembly"]:
            return {"unresolved"}
        for node in walk(statement):
            if node["nodeType"] in ["Assignment", "UnaryOperation"]:
                # Never use a frozen storage value after a possible mutation.
                return {"unresolved"}
            if node["nodeType"] != "FunctionCall":
                continue
            expression = node.get("expression", {})
            builtin = expression.get("typeDescriptions", {}).get("typeIdentifier", "")
            if builtin.startswith("t_function_require_") and self.scalar(node["arguments"][0], aliases) is False:
                if len(node["arguments"]) > 1 and node["arguments"][1].get("kind") == "string":
                    self.reasons.add(node["arguments"][1]["value"])
                return {"blocked"}
            if builtin.startswith("t_function_revert_"):
                if node.get("arguments") and node["arguments"][0].get("kind") == "string":
                    self.reasons.add(node["arguments"][0]["value"])
                return {"blocked"}
            callee = self.nodes.get(expression.get("referencedDeclaration"), {})
            if callee.get("nodeType") == "FunctionDefinition" and callee.get("stateMutability") not in ["view", "pure"]:
                return {"unresolved"}
        return {"continue"}

    def is_self(self, expression):
        if expression.get("nodeType") == "FunctionCall" and expression.get("kind") == "typeConversion":
            return self.is_self(expression["arguments"][0])
        return expression.get("nodeType") == "Identifier" and expression.get("name") == "this" and expression.get("referencedDeclaration") not in self.nodes

    def incoming(self, call, aliases):
        args = call.get("arguments", [])
        return len(args) >= 3 and self.value(args[0], aliases) == {"kind": "sender"} and self.is_self(args[1])

    @staticmethod
    def expression_identity(expression):
        # Ignore source offsets and compiler-generated IDs; retain declarations.
        if isinstance(expression, list):
            return [CurrentState.expression_identity(x) for x in expression]
        if isinstance(expression, dict):
            return {k: CurrentState.expression_identity(v) for k, v in expression.items()
                    if k not in ["id", "src", "typeDescriptions", "nameLocations", "memberLocation", "argumentTypes"]}
        return expression

    def compile_plan(self):
        routes, probes, unresolved = [], [], []
        inventory = []
        reachable = set()

        def visit(function):
            if function["id"] in reachable:
                return
            reachable.add(function["id"])
            for modifier in function.get("modifiers", []):
                related = self.nodes.get(modifier["modifierName"].get("referencedDeclaration"))
                if related and related.get("body"):
                    visit(related)
            for n in walk(function.get("body", {})):
                if n["nodeType"] == "FunctionCall":
                    callee = self.nodes.get(n.get("expression", {}).get("referencedDeclaration"), {})
                    if callee.get("body"):
                        visit(callee)

        for function in self.functions:
            if function.get("visibility") in ["public", "external"] and function.get("kind") != "constructor":
                visit(function)
        all_functions = [self.nodes[x] for x in sorted(reachable)]
        for function in all_functions:
            aliases = self.aliases(function)
            body = function.get("body", {})
            calls = [n for n in walk(body) if n["nodeType"] == "FunctionCall"]
            for n in walk(body):
                if n["nodeType"] == "InlineAssembly" and function["id"] in {f["id"] for f in self.functions}:
                    unresolved.append("custodian_inline_assembly")
            incoming_calls = []
            for call in calls:
                expression = call.get("expression", {})
                if expression.get("nodeType") != "MemberAccess":
                    continue
                callee = self.nodes.get(expression.get("referencedDeclaration"), {})
                selector = callee.get("functionSelector")
                if expression.get("memberName") in ["delegatecall", "callcode", "selfdestruct"]:
                    unresolved.append("dynamic_code_execution")
                if not callee.get("body") and callee.get("nodeType") == "FunctionDefinition":
                    inventory.append({"function": function["name"], "callee": callee["name"], "selector": selector, "mutability": callee.get("stateMutability"), "source_location": call["src"]})
                if selector not in ASSET_OPERATIONS:
                    continue
                operation = ASSET_OPERATIONS[selector]
                row = {"entry_selector": function.get("functionSelector"), "name_observed": function["name"], "operation": operation, "source_location": call["src"]}
                target = self.value(expression["expression"], aliases)
                if operation in ["nft_transfer", "transfer"] and self.incoming(call, aliases):
                    incoming_calls.append(call)
                    row["assessment"] = "incoming_transfer_requires_current_owner_as_from"
                elif target and target["kind"] == "record_field":
                    self.scope = (target["variable"], target["index"])
                    self.reasons = set()
                    paths = self.paths(body, call["id"], aliases)
                    row["control_flow_outcomes"] = sorted(paths)
                    if paths == {"blocked"}:
                        row["assessment"] = "blocked_by_current_state"
                        probes.append({"selector": function.get("functionSelector"), "operation": operation,
                                       "expected_revert_reasons": sorted(self.reasons)})
                    else:
                        row["assessment"] = "unresolved"
                        unresolved.append("exit_not_proven_blocked:" + str(function.get("functionSelector")))
                elif operation == "lp_decrease" and incoming_calls:
                    incoming = incoming_calls[-1]
                    # The later decrease must use the exact manager and NFT
                    # expression passed to the earlier incoming transfer.
                    args = call.get("arguments", [])
                    constructor_args = args[0].get("arguments", []) if args else []
                    same_manager = self.expression_identity(expression["expression"]) == self.expression_identity(incoming["expression"]["expression"])
                    same_nft = constructor_args and self.expression_identity(constructor_args[0]) == self.expression_identity(incoming["arguments"][2])
                    dependencies = {n.get("referencedDeclaration") for expression in [incoming["expression"]["expression"], incoming["arguments"][2]] for n in walk(expression)} - {None}
                    mutated = any(n["nodeType"] == "Assignment" and any(v.get("referencedDeclaration") in dependencies for v in walk(n["leftHandSide"])) for n in walk(body))
                    # Only accept an unconditional earlier top-level transfer.
                    top_level = [s.get("expression", {}).get("id") for s in body.get("statements", [])]
                    if same_manager and same_nft and not mutated and incoming["id"] in top_level and int(incoming["src"].split(":")[0]) < int(call["src"].split(":")[0]):
                        row["assessment"] = "existing_custody_excluded_by_incoming_transfer"
                    else:
                        row["assessment"] = "unresolved"
                        unresolved.append("incoming_guard_not_proven")
                else:
                    row["assessment"] = "unresolved"
                    unresolved.append("asset_target_unsupported")
                routes.append(row)
        if not routes or not probes:
            unresolved.append("blocked_exit_evidence_missing")
        return {"routes": routes, "probe_entries": probes, "external_call_inventory": inventory,
                "reachable_function_count": len(reachable), "unresolved": sorted(set(unresolved)),
                "scope": "observed_configuration_without_configuration_changes",
                "review_mode": "source_derived_direct_exits_and_manual_remaining_call_graph_review",
                "arbitrary_contract_certification": False}


def percentage(locked, total):
    numerator, denominator = int(locked), int(total)
    if denominator <= 0 or numerator < 0 or numerator > denominator:
        raise ValueError("principal_amount_out_of_range")
    value = Fraction(numerator * 100, denominator)
    return str(value.numerator) if value.denominator == 1 else f"{value.numerator}/{value.denominator}"


def finish(discovery, observed, plan, probes):
    for field in ["pool", "block_number", "block_hash", "custodian", "token_id"]:
        if discovery[field] != observed[field] or discovery[field] != probes[field]:
            raise ValueError("observation_identity_mismatch:" + field)
    if plan["unresolved"] or not discovery["complete_single_position_coverage"]:
        raise ValueError("current_configuration_unresolved")
    expected = {p["selector"] for p in plan["probe_entries"]}
    actual = {p["selector"] for p in probes["exit_checks"] if p["reverted"]}
    if actual != expected or not probes["runtime_unchanged"] or not probes["custody_unchanged"]:
        raise ValueError("exit_verification_incomplete")
    if not probes["incoming_existing_nft_reverted"] or not probes["direct_decrease_reverted"] or not probes["erc20_selector_reverted"]:
        raise ValueError("position_manager_bypass_unresolved")
    # No USD weighting or sum of differently ranged raw liquidity values.
    # This bounded sample proves one NFT accounts for ALL pool principal.
    ratios = {token: percentage(discovery["principal_" + token], discovery["principal_" + token])
              for token in ["token0", "token1"] if int(discovery["principal_" + token]) > 0}
    if not ratios or len(set(ratios.values())) != 1:
        raise ValueError("aggregate_percentage_unsupported")
    result = dict(observed)
    result.update({"locked_liquidity_percentage": next(iter(ratios.values())),
                   "principal_percentages": ratios,
                   "pool_principal": {t: discovery["principal_" + t] for t in ["token0", "token1"]},
                   "locked_principal": {t: discovery["principal_" + t] for t in ["token0", "token1"]},
                   "scope": plan["scope"], "review_mode": plan["review_mode"],
                   "arbitrary_contract_certification": False,
                   "unresolved": [], "future_configuration_changes_excluded": True})
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--finish", action="store_true")
    args = parser.parse_args()
    read = lambda name: json.loads((args.directory / name).read_text())
    discovery, observed = read("discovery.json"), read("independent-result.json")
    if args.finish:
        plan, probes = read("current-plan.json"), read("current-probes.json")
        review = read("source-review.json")
        # This is a review of THIS discovered source after discovery, not a
        # preloaded custodian registry. Never silently treat it as automation.
        for name in ["source.json", "current-plan.json"]:
            digest = hashlib.sha256((args.directory / name).read_bytes()).hexdigest()
            if review.get("input_sha256", {}).get(name) != digest:
                raise ValueError("manual_source_review_mismatch:" + name)
        required_reviews = {"remaining_external_calls", "storage_writes", "callbacks_and_modifiers", "operator_approvals", "incoming_transfer_scope"}
        if set(review.get("completed_checks", [])) != required_reviews or review.get("scope") != plan["scope"]:
            raise ValueError("manual_source_review_incomplete")
        result = finish(discovery, observed, plan, probes)
        for name in ["discovery.json", "independent-result.json", "current-plan.json", "current-probes.json", "source-review.json"]:
            result.setdefault("input_sha256", {})[name] = hashlib.sha256((args.directory / name).read_bytes()).hexdigest()
        output = "current-result.json"
    else:
        result = CurrentState(read("compiled.json"), read("source.json"), discovery, observed).compile_plan()
        output = "current-plan.json"
    (args.directory / output).write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
