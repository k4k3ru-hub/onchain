"""Generate a bounded structural analysis plan without a locker registry.

Only ERC-721 / Uniswap position-manager selectors are predefined. Custodian
names, ABI methods, field names, storage slots and record IDs come from inputs.
This extracts evidence; it deliberately does not certify all withdrawal paths.
"""

import argparse
import copy
import hashlib
import json
from pathlib import Path

ASSET_OPERATIONS = {
    "42842e0e": "nft_transfer", "b88d4fde": "nft_transfer",
    "23b872dd": "transfer", "095ea7b3": "approval",
    "a22cb465": "operator_approval", "0c49ccbe": "lp_decrease",
}


def walk(value):
    if isinstance(value, dict):
        if "nodeType" in value:
            yield value
        for item in value.values():
            yield from walk(item)
    elif isinstance(value, list):
        for item in value:
            yield from walk(item)


class Analyzer:
    def __init__(self, compiled, source):
        self.nodes = {n["id"]: n for n in walk(compiled["sources"]) if "id" in n}
        file_name, contract_name = source["compilation"]["fullyQualifiedName"].rsplit(":", 1)
        self.main = next(n for n in compiled["sources"][file_name]["ast"]["nodes"] if n.get("nodeType") == "ContractDefinition" and n["name"] == contract_name)
        self.contract = compiled["contracts"][file_name][contract_name]
        self.storage = self.contract["storageLayout"]
        self.variables = {n["astId"]: n for n in self.storage["storage"]}
        self.functions = [n for cid in self.main["linearizedBaseContracts"] for n in self.nodes[cid].get("nodes", []) if n.get("nodeType") == "FunctionDefinition" and n.get("body")]

    def aliases(self, function):
        aliases = {}
        for node in walk(function.get("body", {})):
            if node["nodeType"] == "VariableDeclarationStatement" and len(node.get("declarations", [])) == 1:
                declaration = node["declarations"][0]
                if declaration and node.get("initialValue"):
                    aliases[declaration["id"]] = node["initialValue"]
        return aliases

    def value(self, expression, aliases, depth=0):
        if not isinstance(expression, dict) or depth > 12:
            return None
        kind = expression.get("nodeType")
        ref = expression.get("referencedDeclaration")
        if kind == "Identifier":
            if ref in aliases:
                return self.value(aliases[ref], aliases, depth + 1)
            if ref in self.variables:
                return {"kind": "state", "variable": ref}
            declaration = self.nodes.get(ref, {})
            if declaration.get("constant") and declaration.get("value"):
                return self.value(declaration["value"], aliases, depth + 1)
            return {"kind": "parameter", "declaration": ref}
        if kind == "IndexAccess":
            base = expression.get("baseExpression", {}).get("referencedDeclaration")
            if base in self.variables and self.storage["types"][self.variables[base]["type"]]["encoding"] == "mapping":
                return {"kind": "record", "variable": base, "index": self.value(expression.get("indexExpression"), aliases, depth + 1)}
        if kind == "MemberAccess":
            inner = expression["expression"]
            if inner.get("typeDescriptions", {}).get("typeIdentifier") == "t_magic_block" and expression["memberName"] == "timestamp":
                return {"kind": "timestamp"}
            if inner.get("typeDescriptions", {}).get("typeIdentifier") == "t_magic_message" and expression["memberName"] == "sender":
                return {"kind": "sender"}
            base = self.value(inner, aliases, depth + 1)
            if base and base["kind"] == "record":
                return {**base, "kind": "record_field", "field": ref}
        if kind == "Literal" and expression.get("kind") == "number":
            return {"kind": "literal", "value": int(expression["value"], 0) if expression["value"].startswith("0x") else int(expression["value"])}
        if kind == "FunctionCall":
            if expression.get("kind") == "typeConversion" and len(expression.get("arguments", [])) == 1:
                return self.value(expression["arguments"][0], aliases, depth + 1)
            function = self.nodes.get(expression.get("expression", {}).get("referencedDeclaration"), {})
            statements = function.get("body", {}).get("statements", [])
            if not expression.get("arguments") and len(statements) == 1 and statements[0].get("nodeType") == "Return":
                return self.value(statements[0].get("expression"), self.aliases(function), depth + 1)
        return None

    def condition(self, node, aliases):
        if node.get("nodeType") != "BinaryOperation":
            return None
        left = self.value(node["leftExpression"], aliases)
        right = self.value(node["rightExpression"], aliases)
        if left and right:
            return {"left": left, "operator": node["operator"], "right": right}
        return None

    def local_requires(self, function):
        result = []
        aliases = self.aliases(function)
        for node in walk(function.get("body", {})):
            builtin = node.get("expression", {}).get("typeDescriptions", {}).get("typeIdentifier", "")
            if node["nodeType"] == "FunctionCall" and builtin.startswith("t_function_require_") and node.get("arguments"):
                condition = self.condition(node["arguments"][0], aliases)
                if condition:
                    result.append(condition)
        return result

    def related_requires(self, function, seen=None):
        seen = set() if seen is None else seen
        if function["id"] in seen:
            return []
        seen.add(function["id"])
        result = self.local_requires(function)
        ids = [m["modifierName"].get("referencedDeclaration") for m in function.get("modifiers", [])]
        for n in walk(function.get("body", {})):
            if n["nodeType"] == "FunctionCall" and n.get("expression", {}).get("nodeType") == "Identifier":
                ids.append(n["expression"].get("referencedDeclaration"))
        for ref in ids:
            related = self.nodes.get(ref)
            if related and related.get("body"):
                result.extend(self.related_requires(related, seen))
        return result

    def field(self, mapping_id, field_id):
        variable = self.variables[mapping_id]
        map_type = self.storage["types"][variable["type"]]
        if self.storage["types"][map_type["key"]]["label"] != "uint256":
            raise ValueError("mapping_key_type_unsupported")
        members = self.storage["types"][map_type["value"]]["members"]
        member = next(m for m in members if m["astId"] == field_id)
        field_type = self.storage["types"][member["type"]]
        if field_type["encoding"] != "inplace" or "members" in field_type:
            raise ValueError("nested_record_field_unsupported")
        return {"mapping_slot": variable["slot"], "slot": member["slot"], "offset": member["offset"], "bytes": int(field_type["numberOfBytes"]), "field_ast_id": field_id}

    def build(self):
        records = []
        exits = []
        for function in self.functions:
            aliases = self.aliases(function)
            requires = self.related_requires(function)
            for call in walk(function.get("body", {})):
                if call["nodeType"] != "FunctionCall" or call.get("expression", {}).get("nodeType") != "MemberAccess":
                    continue
                method = call["expression"]
                callee = self.nodes.get(method.get("referencedDeclaration"), {})
                selector = callee.get("functionSelector")
                if selector not in ASSET_OPERATIONS:
                    continue
                target = self.value(method["expression"], aliases)
                args = [self.value(arg, aliases) for arg in call.get("arguments", [])]
                row = {"entry_name_observed": function["name"], "entry_selector": function.get("functionSelector"), "operation": ASSET_OPERATIONS[selector], "target": target, "arguments": args, "requires_found": requires, "source_location": call["src"]}
                exits.append(row)
                # An NFT exit whose manager and token ID come from one record.
                if row["operation"] not in ["nft_transfer", "transfer"] or len(args) < 3 or not target or not args[2]:
                    continue
                token = args[2]
                if target["kind"] != "record_field" or token["kind"] != "record_field" or target["variable"] != token["variable"] or target["index"] != token["index"]:
                    continue
                mapping = target["variable"]
                time_fields = []
                owner_fields = []
                for condition in requires:
                    left, right = condition["left"], condition["right"]
                    if left["kind"] == "record_field" and left["variable"] == mapping and right["kind"] == "timestamp" and condition["operator"] in ["<", "<="]:
                        time_fields.append({**self.field(mapping, left["field"]), "comparison": condition["operator"]})
                    for value, other in [(left, right), (right, left)]:
                        if value["kind"] == "record_field" and value["variable"] == mapping and other["kind"] == "sender" and condition["operator"] == "==":
                            owner_fields.append(self.field(mapping, value["field"]))
                if not time_fields:
                    continue
                records.append({"entry_selector": function.get("functionSelector"), "manager_field": self.field(mapping, target["field"]), "nft_field": self.field(mapping, token["field"]), "time_fields": time_fields, "owner_fields": owner_fields, "guard_dominance_proven": False})

        mutable_gates = []
        for exit in exits:
            if exit["operation"] not in ["approval", "operator_approval"]:
                continue
            for condition in exit["requires_found"]:
                left, right = condition["left"], condition["right"]
                if left["kind"] != "state" or right != {"kind": "literal", "value": 0} or condition["operator"] != "!=":
                    continue
                variable_id = left["variable"]
                variable = self.variables[variable_id]
                writers = []
                for function in self.functions:
                    if function.get("visibility") not in ["public", "external"]:
                        continue
                    writes = [n for n in walk(function.get("body", {})) if n["nodeType"] == "Assignment" and n.get("leftHandSide", {}).get("referencedDeclaration") == variable_id]
                    if writes:
                        roles = []
                        for c in self.related_requires(function):
                            for value, other in [(c["left"], c["right"]), (c["right"], c["left"])]:
                                if value["kind"] == "state" and other["kind"] == "sender" and c["operator"] == "==":
                                    roles.append(copy.deepcopy(self.variables[value["variable"]]))
                        writers.append({"selector": function.get("functionSelector"), "name_observed": function["name"], "role_storage": roles})
                mutable_gates.append({"slot": variable["slot"], "offset": variable["offset"], "bytes": int(self.storage["types"][variable["type"]]["numberOfBytes"]), "writers": writers, "exit_selector": exit["entry_selector"]})

        return {"records": records, "asset_operations": exits, "mutable_gates": mutable_gates,
                "locked_liquidity_percentage": None,
                "unresolved": ["all_path_control_flow_and_interprocedural_asset_effects_not_proven"],
                "public_entry_count": sum(f.get("visibility") in ["external", "public"] for f in self.functions)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    source = json.loads((args.directory / "source.json").read_text())
    compiled = json.loads((args.directory / "compiled.json").read_text())
    analysis = Analyzer(compiled, source).build()
    analysis["source_input_sha256"] = hashlib.sha256((args.directory / "source.json").read_bytes()).hexdigest()
    (args.directory / "analysis-plan.json").write_text(json.dumps(analysis, indent=2) + "\n")
    print(json.dumps({"record_shapes": len(analysis["records"]), "asset_operations": len(analysis["asset_operations"]), "mutable_gates": len(analysis["mutable_gates"]), "locked_liquidity_percentage": None}))


if __name__ == "__main__":
    main()
