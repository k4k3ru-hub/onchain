// SPDX-License-Identifier: MIT
pragma solidity 0.8.30;

// Research fixtures only. None of these contracts is deployed to a public chain.
contract Plain {
    string public constant name = "Tax validation";
    string public constant symbol = "TEST";
    uint8 public constant decimals = 18;
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;
    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
    constructor() {
        totalSupply = 10**27;
        balanceOf[msg.sender] = totalSupply;
        emit Transfer(address(0), msg.sender, totalSupply);
    }
    function approve(address spender, uint256 value) external returns (bool) {
        allowance[msg.sender][spender] = value;
        emit Approval(msg.sender, spender, value);
        return true;
    }
    function transfer(address to, uint256 value) external returns (bool) {
        _move(msg.sender, to, value);
        return true;
    }
    function transferFrom(address from, address to, uint256 value) external returns (bool) {
        uint256 remaining = allowance[from][msg.sender];
        if (remaining != type(uint256).max) allowance[from][msg.sender] = remaining - value;
        _move(from, to, value);
        return true;
    }
    function _move(address from, address to, uint256 value) internal virtual {
        balanceOf[from] -= value;
        balanceOf[to] += value;
        emit Transfer(from, to, value);
    }
}

contract PoolTax is Plain {
    address public controller;
    uint256 public buyBps = 200;
    uint256 public sellBps = 500;
    mapping(address => bool) public taxedPools;
    mapping(address => bool) public exempt;
    constructor() { controller = msg.sender; }
    function setRates(uint256 buy, uint256 sell) external {
        require(msg.sender == controller && buy <= 10000 && sell <= 10000);
        buyBps = buy;
        sellBps = sell;
    }
    function setPool(address pool, bool enabled) external {
        require(msg.sender == controller);
        taxedPools[pool] = enabled;
    }
    function setExempt(address account, bool enabled) external {
        require(msg.sender == controller);
        exempt[account] = enabled;
    }
    function _move(address from, address to, uint256 value) internal override {
        uint256 fee;
        if (!exempt[from] && !exempt[to]) {
            uint256 rate = taxedPools[from] ? buyBps : (taxedPools[to] ? sellBps : 0);
            fee = value * rate / 10000;
        }
        if (fee > 0) super._move(from, address(0xdead), fee);
        super._move(from, to, value - fee);
    }
}

contract RoleTax is PoolTax {
    // A zero owner getter does not remove the separate controller's authority.
    function owner() external pure returns (address) { return address(0); }
}

contract FakeZero is Plain {
    function buyTax() external pure returns (uint256) { return 0; }
    function sellTax() external pure returns (uint256) { return 0; }
    function _move(address from, address to, uint256 value) internal override {
        uint256 fee = value * 9 / 100;
        super._move(from, address(0xdead), fee);
        super._move(from, to, value - fee);
    }
}

contract ComplexTax is Plain {
    function _move(address from, address to, uint256 value) internal override {
        uint256 fee = value == 0 ? 0 : 1 + value / 100;
        super._move(from, address(0xdead), fee);
        super._move(from, to, value - fee);
    }
}

contract StandardProxy {
    bytes32 private constant IMPL = 0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc;
    bytes32 private constant ADMIN = 0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103;
    constructor(address implementation) {
        assembly { sstore(IMPL, implementation) sstore(ADMIN, caller()) }
    }
    function upgradeTo(address implementation) external {
        assembly { if iszero(eq(caller(), sload(ADMIN))) { revert(0, 0) } sstore(IMPL, implementation) }
    }
    fallback() external {
        assembly {
            calldatacopy(0, 0, calldatasize())
            let ok := delegatecall(gas(), sload(IMPL), 0, calldatasize(), 0, 0)
            returndatacopy(0, 0, returndatasize())
            if iszero(ok) { revert(0, returndatasize()) }
            return(0, returndatasize())
        }
    }
}

contract UnknownProxy {
    address private target;
    constructor(address implementation) { target = implementation; }
    fallback() external {
        assembly {
            calldatacopy(0, 0, calldatasize())
            let ok := delegatecall(gas(), sload(0), 0, calldatasize(), 0, 0)
            returndatacopy(0, 0, returndatasize())
            if iszero(ok) { revert(0, returndatasize()) }
            return(0, returndatasize())
        }
    }
}
