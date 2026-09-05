// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ISpreadExecutor} from "./ISpreadExecutor.sol";

interface IERC20SpreadExecutor {
    function approve(address spender, uint256 amount) external returns (bool);
    function balanceOf(address account) external view returns (uint256);
    function transfer(address recipient, uint256 amount) external returns (bool);
    function transferFrom(address sender, address recipient, uint256 amount) external returns (bool);
}

contract SpreadExecutor is ISpreadExecutor {
    mapping(bytes32 callKey => bool allowed) private allowedBuyCalls;
    mapping(bytes32 callKey => bool allowed) private allowedSellCalls;
    bool private entered;

    constructor(
        address[] memory buyTargets,
        bytes4[] memory buySelectors,
        address[] memory sellTargets,
        bytes4[] memory sellSelectors
    ) {
        if (
            buyTargets.length == 0 || sellTargets.length == 0 || buyTargets.length != buySelectors.length
                || sellTargets.length != sellSelectors.length
        ) revert LengthMismatch();
        _allowCalls(allowedBuyCalls, buyTargets, buySelectors);
        _allowCalls(allowedSellCalls, sellTargets, sellSelectors);
    }

    function _allowCalls(
        mapping(bytes32 callKey => bool allowed) storage calls,
        address[] memory targets,
        bytes4[] memory selectors
    ) private {
        for (uint256 i; i < targets.length; ++i) {
            if (targets[i] == address(0) || targets[i].code.length == 0 || selectors[i] == bytes4(0)) {
                revert InvalidAddress();
            }
            calls[_callKey(targets[i], selectors[i])] = true;
        }
    }

    modifier nonReentrant() {
        if (entered) revert ReentrantCall();
        entered = true;
        _;
        entered = false;
    }

    function executeSpread(
        Execution calldata execution
    ) external nonReentrant returns (uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit) {
        _validateExecution(execution);

        IERC20SpreadExecutor baseToken = IERC20SpreadExecutor(execution.baseToken);
        IERC20SpreadExecutor quoteToken = IERC20SpreadExecutor(execution.quoteToken);
        uint256 initialBaseBalance = baseToken.balanceOf(address(this));
        uint256 initialQuoteBalance = quoteToken.balanceOf(address(this));

        _safeTransferFrom(quoteToken, msg.sender, address(this), execution.maximumQuoteIn);
        quoteSpent = _executeBuy(execution, baseToken, quoteToken, initialBaseBalance);
        (quoteReceived, quoteProfit) = _executeSellAndSettle(
            execution,
            baseToken,
            quoteToken,
            initialBaseBalance,
            initialQuoteBalance,
            quoteSpent
        );
    }

    function _executeBuy(
        Execution calldata execution,
        IERC20SpreadExecutor baseToken,
        IERC20SpreadExecutor quoteToken,
        uint256 initialBaseBalance
    ) private returns (uint256 quoteSpent) {
        uint256 quoteBalanceBeforeBuy = quoteToken.balanceOf(address(this));
        _safeApprove(quoteToken, execution.buy.target, execution.maximumQuoteIn);
        _executeVenueCall(execution.buy);
        _safeApprove(quoteToken, execution.buy.target, 0);

        uint256 quoteBalanceAfterBuy = quoteToken.balanceOf(address(this));
        if (quoteBalanceAfterBuy > quoteBalanceBeforeBuy) revert InvalidAmount();
        quoteSpent = quoteBalanceBeforeBuy - quoteBalanceAfterBuy;

        uint256 baseBalanceAfterBuy = baseToken.balanceOf(address(this));
        uint256 baseBought = baseBalanceAfterBuy - initialBaseBalance;
        if (baseBought < execution.baseAmount) {
            revert InsufficientBaseBought(baseBought, execution.baseAmount);
        }
    }

    function _executeSellAndSettle(
        Execution calldata execution,
        IERC20SpreadExecutor baseToken,
        IERC20SpreadExecutor quoteToken,
        uint256 initialBaseBalance,
        uint256 initialQuoteBalance,
        uint256 quoteSpent
    ) private returns (uint256 quoteReceived, uint256 quoteProfit) {
        uint256 quoteBalanceAfterBuy = quoteToken.balanceOf(address(this));
        _safeApprove(baseToken, execution.sell.target, execution.baseAmount);
        _executeVenueCall(execution.sell);
        _safeApprove(baseToken, execution.sell.target, 0);

        uint256 finalQuoteBalance = quoteToken.balanceOf(address(this));
        if (finalQuoteBalance < quoteBalanceAfterBuy) revert InvalidAmount();
        quoteReceived = finalQuoteBalance - quoteBalanceAfterBuy;
        if (quoteReceived < execution.minimumQuoteOut) {
            revert InsufficientQuoteOutput(quoteReceived, execution.minimumQuoteOut);
        }
        if (quoteReceived < quoteSpent) revert InsufficientQuoteProfit(0, execution.minimumQuoteProfit);
        quoteProfit = quoteReceived - quoteSpent;
        if (quoteProfit < execution.minimumQuoteProfit) {
            revert InsufficientQuoteProfit(quoteProfit, execution.minimumQuoteProfit);
        }

        uint256 quoteRefund = finalQuoteBalance - initialQuoteBalance;
        if (quoteRefund > 0) _safeTransfer(quoteToken, msg.sender, quoteRefund);
        uint256 finalBaseBalance = baseToken.balanceOf(address(this));
        if (finalBaseBalance > initialBaseBalance) {
            _safeTransfer(baseToken, msg.sender, finalBaseBalance - initialBaseBalance);
        }

        emit SpreadExecuted(
            msg.sender,
            execution.baseToken,
            execution.quoteToken,
            execution.baseAmount,
            quoteSpent,
            quoteReceived,
            quoteProfit
        );
    }

    function isCallAllowed(Side side, address target, bytes4 selector) external view returns (bool) {
        if (side == Side.Buy) return allowedBuyCalls[_callKey(target, selector)];
        return allowedSellCalls[_callKey(target, selector)];
    }

    function _validateExecution(Execution calldata execution) private view {
        if (
            execution.baseToken == address(0) || execution.quoteToken == address(0)
                || execution.buy.target == address(0) || execution.sell.target == address(0)
        ) revert InvalidAddress();
        if (execution.baseToken == execution.quoteToken) revert InvalidAddress();
        if (execution.buy.target == execution.sell.target) revert DuplicateTarget(execution.buy.target);
        if (
            execution.baseAmount == 0 || execution.maximumQuoteIn == 0 || execution.minimumQuoteOut == 0
        ) revert InvalidAmount();
        if (execution.deadline < block.timestamp) revert DeadlineExpired(execution.deadline, block.timestamp);
        _validateVenueCall(Side.Buy, execution.buy);
        _validateVenueCall(Side.Sell, execution.sell);
    }

    function _validateVenueCall(Side side, VenueCall calldata venueCall) private view {
        if (venueCall.data.length < 4) revert InvalidCallData();
        bytes4 selector = bytes4(venueCall.data[:4]);
        bool allowed = side == Side.Buy
            ? allowedBuyCalls[_callKey(venueCall.target, selector)]
            : allowedSellCalls[_callKey(venueCall.target, selector)];
        if (!allowed) {
            revert CallNotAllowed(venueCall.target, selector);
        }
    }

    function _executeVenueCall(VenueCall calldata venueCall) private {
        (bool success, bytes memory reason) = venueCall.target.call(venueCall.data);
        if (!success) revert VenueCallFailed(venueCall.target, reason);
    }

    function _safeApprove(IERC20SpreadExecutor token, address spender, uint256 amount) private {
        _callToken(token, abi.encodeCall(token.approve, (spender, amount)));
    }

    function _safeTransfer(IERC20SpreadExecutor token, address recipient, uint256 amount) private {
        _callToken(token, abi.encodeCall(token.transfer, (recipient, amount)));
    }

    function _safeTransferFrom(
        IERC20SpreadExecutor token,
        address sender,
        address recipient,
        uint256 amount
    ) private {
        _callToken(token, abi.encodeCall(token.transferFrom, (sender, recipient, amount)));
    }

    function _callToken(IERC20SpreadExecutor token, bytes memory data) private {
        (bool success, bytes memory result) = address(token).call(data);
        if (!success || (result.length != 0 && !abi.decode(result, (bool)))) {
            revert TokenCallFailed(address(token));
        }
    }

    function _callKey(address target, bytes4 selector) private pure returns (bytes32) {
        return keccak256(abi.encode(target, selector));
    }
}
