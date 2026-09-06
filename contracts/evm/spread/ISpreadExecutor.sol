// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface ISpreadExecutor {
    enum Side {
        Buy,
        Sell
    }

    struct VenueCall {
        address target;
        bytes data;
    }

    struct Execution {
        address baseToken;
        address quoteToken;
        uint256 baseAmount;
        uint256 maximumQuoteIn;
        uint256 minimumQuoteOut;
        uint256 minimumQuoteProfit;
        uint256 deadline;
        VenueCall buy;
        VenueCall sell;
    }

    error CallNotAllowed(address target, bytes4 selector);
    error DeadlineExpired(uint256 deadline, uint256 timestamp);
    error DuplicateTarget(address target);
    error InsufficientBaseBought(uint256 actualAmount, uint256 requiredAmount);
    error InsufficientQuoteOutput(uint256 actualAmount, uint256 minimumAmount);
    error InsufficientQuoteProfit(uint256 actualAmount, uint256 minimumAmount);
    error InvalidAddress();
    error InvalidAmount();
    error InvalidCallData();
    error LengthMismatch();
    error NotAdmin(address account);
    error NotPauser(address account);
    error Paused();
    error NotPaused();
    error ReentrantCall();
    error TokenCallFailed(address token);
    error VenueCallFailed(address target, bytes reason);

    event SpreadExecuted(
        address indexed account,
        address indexed baseToken,
        address indexed quoteToken,
        uint256 baseAmount,
        uint256 quoteSpent,
        uint256 quoteReceived,
        uint256 quoteProfit
    );
    event AdminTransferStarted(address indexed currentAdmin, address indexed pendingAdmin);
    event AdminTransferred(address indexed previousAdmin, address indexed newAdmin);
    event PauserChanged(address indexed previousPauser, address indexed newPauser);
    event ExecutorPaused(address indexed account);
    event ExecutorUnpaused(address indexed account);
    event TokenRescued(address indexed token, address indexed recipient, uint256 amount);

    function executeSpread(Execution calldata execution)
        external
        returns (uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit);

    function isCallAllowed(Side side, address target, bytes4 selector) external view returns (bool);

    function admin() external view returns (address);

    function pendingAdmin() external view returns (address);

    function pauser() external view returns (address);

    function paused() external view returns (bool);

    function pause() external;

    function unpause() external;

    function setPauser(address newPauser) external;

    function transferAdmin(address newAdmin) external;

    function acceptAdmin() external;

    function rescueToken(address token, address recipient, uint256 amount) external;
}
