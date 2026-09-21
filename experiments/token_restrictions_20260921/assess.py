"""Classify the bounded experiment evidence, without asserting token safety."""


def succeeded(response):
    return isinstance(response, dict) and "result" in response and "error" not in response


def reverted(response):
    return isinstance(response, dict) and response.get("error", {}).get("code") == 3


def token_transfer_reverted(trace, token):
    if not succeeded(trace):
        return False

    def visit(call):
        if (call.get("to", "").lower() == token.lower()
                and call.get("input", "").startswith("0x23b872dd")
                and call.get("error") == "execution reverted"):
            return True
        return any(visit(child) for child in call.get("calls", []))

    return visit(trace["result"])


def assess(ordinary, owner, token):
    """Return supported observations only; missing/failed data stays inconclusive."""
    quantity = int(ordinary.get("receivedRaw", "0"))
    bought = ordinary.get("buyReceipt", {}).get("status") == "0x1" and quantity > 0
    funded = int(ordinary.get("balanceBeforeSellRaw", "0")) >= quantity > 0
    approved = int(ordinary.get("allowanceBeforeSellRaw", "0")) >= quantity > 0
    rejected = (bought and funded and approved
                and reverted(ordinary.get("sellNextBlock"))
                and ordinary.get("sellReceipt", {}).get("status") == "0x0"
                and token_transfer_reverted(ordinary.get("sellTraceNextBlock"), token))
    flag_explains_rejection = any(
        row.get("balanceUnchanged") is True and succeeded(row.get("sellWithOnlySlotCleared"))
        for row in ordinary.get("restrictionSlotCounterfactuals", []))
    automatic = (rejected and succeeded(ordinary.get("sellSameBlock"))
                 and flag_explains_rejection)
    allowlist = (rejected and reverted(ordinary.get("allowlistOrdinaryCall"))
                 and ordinary.get("allowlistOwnerReceipt", {}).get("status") == "0x1"
                 and succeeded(ordinary.get("sellAfterAllowlist"))
                 and ordinary.get("sellAfterAllowlistReceipt", {}).get("status") == "0x1")
    owner_sold = (owner.get("buyReceipt", {}).get("status") == "0x1"
                  and int(owner.get("receivedRaw", "0")) > 0
                  and succeeded(owner.get("sellNextBlock"))
                  and owner.get("sellReceipt", {}).get("status") == "0x1")
    return {
        "scope": "historical_fork_selected_token_route_amount_and_addresses",
        "buyThenSellRejectedByToken": "observed" if rejected else "inconclusive",
        "automaticRecipientRestriction": "observed" if automatic else "inconclusive",
        "ownerControlledAllowlistBypass": "observed" if allowlist else "inconclusive",
        "ownerRoundTripSucceeded": "observed" if owner_sold else "inconclusive",
        "sellStillRejectedAfter65Blocks": "observed" if reverted(ordinary.get("sellAfter65Blocks")) else "inconclusive",
        "safetyVerdict": "not_provided",
    }
