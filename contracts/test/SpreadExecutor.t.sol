// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ISpreadExecutor} from "../evm/spread/ISpreadExecutor.sol";
import {SpreadExecutor} from "../evm/spread/SpreadExecutor.sol";

contract MockERC20 {
    mapping(address account => uint256 balance) public balanceOf;
    mapping(address owner => mapping(address spender => uint256 amount)) public allowance;

    function mint(address account, uint256 amount) external {
        balanceOf[account] += amount;
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        return true;
    }

    function transfer(address recipient, uint256 amount) external returns (bool) {
        _transfer(msg.sender, recipient, amount);
        return true;
    }

    function transferFrom(address sender, address recipient, uint256 amount) external returns (bool) {
        uint256 available = allowance[sender][msg.sender];
        require(available >= amount, "allowance");
        allowance[sender][msg.sender] = available - amount;
        _transfer(sender, recipient, amount);
        return true;
    }

    function _transfer(address sender, address recipient, uint256 amount) private {
        require(balanceOf[sender] >= amount, "balance");
        balanceOf[sender] -= amount;
        balanceOf[recipient] += amount;
    }
}

contract MockBuyRouter {
    function buyExactOutput(
        address quoteToken,
        address baseToken,
        uint256 baseAmount,
        uint256 quoteSpent,
        address recipient
    ) external {
        MockERC20(quoteToken).transferFrom(msg.sender, address(this), quoteSpent);
        MockERC20(baseToken).transfer(recipient, baseAmount);
    }
}

contract MockSellRouter {
    function sellExactInput(
        address baseToken,
        address quoteToken,
        uint256 baseAmount,
        uint256 quoteReceived,
        address recipient
    ) external {
        MockERC20(baseToken).transferFrom(msg.sender, address(this), baseAmount);
        MockERC20(quoteToken).transfer(recipient, quoteReceived);
    }
}

contract SpreadExecutorCaller {
    function pause(SpreadExecutor executor) external {
        executor.pause();
    }

    function unpause(SpreadExecutor executor) external {
        executor.unpause();
    }

    function acceptAdmin(SpreadExecutor executor) external {
        executor.acceptAdmin();
    }
}

contract SpreadExecutorTest {
    MockERC20 private baseToken;
    MockERC20 private quoteToken;
    MockBuyRouter private buyRouter;
    MockSellRouter private sellRouter;
    SpreadExecutor private executor;

    constructor() {
        baseToken = new MockERC20();
        quoteToken = new MockERC20();
        buyRouter = new MockBuyRouter();
        sellRouter = new MockSellRouter();

        address[] memory buyTargets = new address[](1);
        bytes4[] memory buySelectors = new bytes4[](1);
        address[] memory sellTargets = new address[](1);
        bytes4[] memory sellSelectors = new bytes4[](1);
        buyTargets[0] = address(buyRouter);
        buySelectors[0] = MockBuyRouter.buyExactOutput.selector;
        sellTargets[0] = address(sellRouter);
        sellSelectors[0] = MockSellRouter.sellExactInput.selector;
        executor =
            new SpreadExecutor(buyTargets, buySelectors, sellTargets, sellSelectors, address(this), address(this));

        baseToken.mint(address(buyRouter), 1_000_000);
        quoteToken.mint(address(sellRouter), 1_000_000);
        quoteToken.mint(address(this), 1_000_000);
        quoteToken.approve(address(executor), type(uint256).max);
    }

    function testExecuteProfitableSpread() external {
        uint256 initialQuoteBalance = quoteToken.balanceOf(address(this));
        ISpreadExecutor.Execution memory execution = _execution(100, 1_000, 1_020, 20);
        (uint256 spent, uint256 received, uint256 profit) = executor.executeSpread(execution);
        require(spent == 1_000, "spent");
        require(received == 1_020, "received");
        require(profit == 20, "profit");
        require(quoteToken.balanceOf(address(this)) == initialQuoteBalance + 20, "refund");
        require(baseToken.balanceOf(address(executor)) == 0, "base dust");
        require(quoteToken.balanceOf(address(executor)) == 0, "quote dust");
    }

    function testRevertsBelowMinimumProfit() external {
        ISpreadExecutor.Execution memory execution = _execution(100, 1_000, 1_010, 20);
        (bool success,) = address(executor).call(abi.encodeCall(executor.executeSpread, (execution)));
        require(!success, "expected revert");
    }

    function testRevertsUnapprovedSelector() external {
        ISpreadExecutor.Execution memory execution = _execution(100, 1_000, 1_020, 20);
        execution.buy.data = abi.encodeCall(
            MockSellRouter.sellExactInput, (address(baseToken), address(quoteToken), 100, 1_020, address(executor))
        );
        (bool success,) = address(executor).call(abi.encodeCall(executor.executeSpread, (execution)));
        require(!success, "expected revert");
    }

    function testPauseBlocksExecutionAndAdminCanUnpause() external {
        executor.pause();
        require(executor.paused(), "paused");
        ISpreadExecutor.Execution memory execution = _execution(100, 1_000, 1_020, 20);
        (bool success,) = address(executor).call(abi.encodeCall(executor.executeSpread, (execution)));
        require(!success, "expected paused revert");
        executor.unpause();
        require(!executor.paused(), "unpaused");
    }

    function testRescueTokenRequiresPausedState() external {
        quoteToken.mint(address(executor), 100);
        (bool success,) =
            address(executor).call(abi.encodeCall(executor.rescueToken, (address(quoteToken), address(this), 100)));
        require(!success, "expected not paused revert");
        executor.pause();
        uint256 balanceBefore = quoteToken.balanceOf(address(this));
        executor.rescueToken(address(quoteToken), address(this), 100);
        require(quoteToken.balanceOf(address(this)) == balanceBefore + 100, "rescued");
        executor.unpause();
    }

    function testPauserCannotUnpause() external {
        SpreadExecutorCaller emergencyPauser = new SpreadExecutorCaller();
        (bool unauthorized,) = address(emergencyPauser).call(abi.encodeCall(emergencyPauser.pause, (executor)));
        require(!unauthorized, "unauthorized pause");
        executor.setPauser(address(emergencyPauser));
        emergencyPauser.pause(executor);
        (bool success,) = address(emergencyPauser).call(abi.encodeCall(emergencyPauser.unpause, (executor)));
        require(!success, "pauser unpaused");
        executor.unpause();
    }

    function testAdminTransferRequiresAcceptance() external {
        SpreadExecutorCaller newAdmin = new SpreadExecutorCaller();
        executor.transferAdmin(address(newAdmin));
        require(executor.admin() == address(this), "admin changed early");
        require(executor.pendingAdmin() == address(newAdmin), "pending admin");
        newAdmin.acceptAdmin(executor);
        require(executor.admin() == address(newAdmin), "admin not changed");
        require(executor.pendingAdmin() == address(0), "pending not cleared");
        (bool success,) = address(executor).call(abi.encodeCall(executor.setPauser, (address(this))));
        require(!success, "old admin retained access");
    }

    function _execution(uint256 baseAmount, uint256 quoteSpent, uint256 quoteReceived, uint256 minimumProfit)
        private
        view
        returns (ISpreadExecutor.Execution memory)
    {
        return ISpreadExecutor.Execution({
            baseToken: address(baseToken),
            quoteToken: address(quoteToken),
            baseAmount: baseAmount,
            maximumQuoteIn: quoteSpent,
            minimumQuoteOut: quoteReceived,
            minimumQuoteProfit: minimumProfit,
            deadline: block.timestamp + 1,
            buy: ISpreadExecutor.VenueCall({
                target: address(buyRouter),
                data: abi.encodeCall(
                    MockBuyRouter.buyExactOutput,
                    (address(quoteToken), address(baseToken), baseAmount, quoteSpent, address(executor))
                )
            }),
            sell: ISpreadExecutor.VenueCall({
                target: address(sellRouter),
                data: abi.encodeCall(
                    MockSellRouter.sellExactInput,
                    (address(baseToken), address(quoteToken), baseAmount, quoteReceived, address(executor))
                )
            })
        });
    }
}
