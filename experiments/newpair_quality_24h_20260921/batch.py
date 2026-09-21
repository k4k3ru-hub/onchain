"""Minimal static-call ABI helpers for the research harness."""
from census import END_BLOCK, RPCFailure, topic

MULTICALL = "0xca11bde05977b3631167028862be2a173976ca11"


def word(value):
    return (value % (1 << 256)).to_bytes(32, "big")


def bytes_value(value):
    return word(len(value)) + value + bytes((-len(value)) % 32)


def call_data(signature, *arguments):
    return topic(signature)[:10] + b"".join(word(int(x, 16) if isinstance(x, str) else x) for x in arguments).hex()


def aggregate_data(calls):
    parts = []
    for address, data in calls:
        if len(address) != 42 or not address.startswith("0x"):
            raise ValueError("call_address_invalid")
        parts.append(word(int(address, 16)) + word(1) + word(96) + bytes_value(bytes.fromhex(data[2:])))
    offset = 32 * len(parts)
    offsets = []
    for part in parts:
        offsets.append(word(offset))
        offset += len(part)
    return "0x82ad56cb" + (word(32) + word(len(parts)) + b"".join(offsets) + b"".join(parts)).hex()


def aggregate_result(value, expected):
    data = bytes.fromhex(value[2:])

    def number(offset):
        if offset < 0 or offset + 32 > len(data):
            raise ValueError("truncated_aggregate_result")
        return int.from_bytes(data[offset:offset + 32], "big")

    start = number(0)
    count = number(start)
    if count != expected:
        raise ValueError("aggregate_count_mismatch")
    result = []
    for i in range(count):
        offset = start + 32 + number(start + 32 + 32 * i)
        success = number(offset)
        if success not in [0, 1]:
            raise ValueError("aggregate_bool_invalid")
        body = offset + number(offset + 32)
        size = number(body)
        if body + 32 + size > len(data):
            raise ValueError("aggregate_bytes_truncated")
        result.append({"success": bool(success), "data": "0x" + data[body + 32:body + 32 + size].hex()})
    return result


def batch(reader, calls, size=60):
    results = []
    for start in range(0, len(calls), size):
        group = calls[start:start + size]
        try:
            response = reader.rpc("eth_call", [{"to": MULTICALL, "data": aggregate_data(group)}, hex(END_BLOCK)])
            results.extend(aggregate_result(response, len(group)))
        except RPCFailure as exc:
            results.extend({"success": False, "data": "0x", "retrieval_failure": str(exc)} for _ in group)
    return results
