// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {ISpreadExecutor} from "../evm/spread/ISpreadExecutor.sol";
import {SpreadExecutor} from "../evm/spread/SpreadExecutor.sol";

interface VmBaseFork {
    function createSelectFork(string calldata urlOrAlias) external returns (uint256 forkId);
    function envOr(string calldata name, string calldata defaultValue) external view returns (string memory value);
    function startPrank(address sender) external;
    function stopPrank() external;
}

interface IERC20BaseFork {
    function approve(address spender, uint256 amount) external returns (bool);
    function balanceOf(address account) external view returns (uint256);
}

interface IAerodromeSlipstreamRouter {
    struct ExactOutputSingleParams {
        address tokenIn;
        address tokenOut;
        int24 tickSpacing;
        address recipient;
        uint256 deadline;
        uint256 amountOut;
        uint256 amountInMaximum;
        uint160 sqrtPriceLimitX96;
    }

    function exactOutputSingle(ExactOutputSingleParams calldata params) external payable returns (uint256 amountIn);
}

interface IUniswapV3SwapRouter02 {
    struct ExactInputSingleParams {
        address tokenIn;
        address tokenOut;
        uint24 fee;
        address recipient;
        uint256 amountIn;
        uint256 amountOutMinimum;
        uint160 sqrtPriceLimitX96;
    }

    function exactInputSingle(ExactInputSingleParams calldata params) external payable returns (uint256 amountOut);
}

contract SpreadExecutorBaseForkTest {
    VmBaseFork private constant vm = VmBaseFork(address(uint160(uint256(keccak256("hevm cheat code")))));

    address private constant WETH = 0x4200000000000000000000000000000000000006;
    address private constant USDC = 0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913;
    address private constant AERODROME_ROUTER = 0xBE6D8f0d05cC4be24d5167a3eF062215bE6D18a5;
    address private constant UNISWAP_V3_ROUTER = 0x2626664c2603336E57B271c5C0b26F421741e481;
    address private constant UNISWAP_WETH_USDC_POOL = 0xd0b53D9277642d899DF5C87A3966A349A798F224;

    function testBaseForkRejectsUnprofitableAerodromeToUniswapRoundTripAtomically() external {
        string memory rpcURL = vm.envOr("BASE_RPC_URL", string(""));
        if (bytes(rpcURL).length == 0) return;
        vm.createSelectFork(rpcURL);

        address[] memory buyTargets = new address[](1);
        bytes4[] memory buySelectors = new bytes4[](1);
        address[] memory sellTargets = new address[](1);
        bytes4[] memory sellSelectors = new bytes4[](1);
        buyTargets[0] = AERODROME_ROUTER;
        buySelectors[0] = IAerodromeSlipstreamRouter.exactOutputSingle.selector;
        sellTargets[0] = UNISWAP_V3_ROUTER;
        sellSelectors[0] = IUniswapV3SwapRouter02.exactInputSingle.selector;
        SpreadExecutor executor = new SpreadExecutor(buyTargets, buySelectors, sellTargets, sellSelectors);

        uint256 maximumQuoteIn = 2_000_000;
        uint256 baseAmount = 100_000_000_000_000;
        vm.startPrank(UNISWAP_WETH_USDC_POOL);
        IERC20BaseFork(USDC).approve(address(executor), maximumQuoteIn);
        uint256 initialQuoteBalance = IERC20BaseFork(USDC).balanceOf(UNISWAP_WETH_USDC_POOL);

        ISpreadExecutor.Execution memory execution = ISpreadExecutor.Execution({
            baseToken: WETH,
            quoteToken: USDC,
            baseAmount: baseAmount,
            maximumQuoteIn: maximumQuoteIn,
            minimumQuoteOut: 1,
            minimumQuoteProfit: 0,
            deadline: block.timestamp + 60,
            buy: ISpreadExecutor.VenueCall({
                target: AERODROME_ROUTER,
                data: abi.encodeCall(
                    IAerodromeSlipstreamRouter.exactOutputSingle,
                    (
                        IAerodromeSlipstreamRouter.ExactOutputSingleParams({
                            tokenIn: USDC,
                            tokenOut: WETH,
                            tickSpacing: 100,
                            recipient: address(executor),
                            deadline: block.timestamp + 60,
                            amountOut: baseAmount,
                            amountInMaximum: maximumQuoteIn,
                            sqrtPriceLimitX96: 0
                        })
                    )
                )
            }),
            sell: ISpreadExecutor.VenueCall({
                target: UNISWAP_V3_ROUTER,
                data: abi.encodeCall(
                    IUniswapV3SwapRouter02.exactInputSingle,
                    (
                        IUniswapV3SwapRouter02.ExactInputSingleParams({
                            tokenIn: WETH,
                            tokenOut: USDC,
                            fee: 500,
                            recipient: address(executor),
                            amountIn: baseAmount,
                            amountOutMinimum: 1,
                            sqrtPriceLimitX96: 0
                        })
                    )
                )
            })
        });

        (bool success,) = address(executor).call(abi.encodeCall(executor.executeSpread, (execution)));
        require(!success, "round trip unexpectedly profitable");
        require(IERC20BaseFork(USDC).balanceOf(UNISWAP_WETH_USDC_POOL) == initialQuoteBalance, "quote balance changed");
        require(IERC20BaseFork(USDC).balanceOf(address(executor)) == 0, "executor retained quote");
        require(IERC20BaseFork(WETH).balanceOf(address(executor)) == 0, "executor retained base");
        vm.stopPrank();
    }
}
