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

    function executeSpread(
        Execution calldata execution
    ) external returns (uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit);

    function isCallAllowed(Side side, address target, bytes4 selector) external view returns (bool);
}
