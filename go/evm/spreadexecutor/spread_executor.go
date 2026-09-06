// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package spreadexecutor

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// ISpreadExecutorExecution is an auto generated low-level Go binding around an user-defined struct.
type ISpreadExecutorExecution struct {
	BaseToken          common.Address
	QuoteToken         common.Address
	BaseAmount         *big.Int
	MaximumQuoteIn     *big.Int
	MinimumQuoteOut    *big.Int
	MinimumQuoteProfit *big.Int
	Deadline           *big.Int
	Buy                ISpreadExecutorVenueCall
	Sell               ISpreadExecutorVenueCall
}

// ISpreadExecutorVenueCall is an auto generated low-level Go binding around an user-defined struct.
type ISpreadExecutorVenueCall struct {
	Target common.Address
	Data   []byte
}

// SpreadExecutorMetaData contains all meta data concerning the SpreadExecutor contract.
var SpreadExecutorMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"address[]\",\"name\":\"buyTargets\",\"type\":\"address[]\"},{\"internalType\":\"bytes4[]\",\"name\":\"buySelectors\",\"type\":\"bytes4[]\"},{\"internalType\":\"address[]\",\"name\":\"sellTargets\",\"type\":\"address[]\"},{\"internalType\":\"bytes4[]\",\"name\":\"sellSelectors\",\"type\":\"bytes4[]\"},{\"internalType\":\"address\",\"name\":\"initialAdmin\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"initialPauser\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"acceptAdmin\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"admin\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"baseToken\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"quoteToken\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"baseAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"maximumQuoteIn\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"minimumQuoteOut\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"minimumQuoteProfit\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"deadline\",\"type\":\"uint256\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"internalType\":\"structISpreadExecutor.VenueCall\",\"name\":\"buy\",\"type\":\"tuple\"},{\"components\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"internalType\":\"structISpreadExecutor.VenueCall\",\"name\":\"sell\",\"type\":\"tuple\"}],\"internalType\":\"structISpreadExecutor.Execution\",\"name\":\"execution\",\"type\":\"tuple\"}],\"name\":\"executeSpread\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"quoteSpent\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"quoteReceived\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"quoteProfit\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"enumISpreadExecutor.Side\",\"name\":\"side\",\"type\":\"uint8\"},{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes4\",\"name\":\"selector\",\"type\":\"bytes4\"}],\"name\":\"isCallAllowed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"pause\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"paused\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"pauser\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"pendingAdmin\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"rescueToken\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newPauser\",\"type\":\"address\"}],\"name\":\"setPauser\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newAdmin\",\"type\":\"address\"}],\"name\":\"transferAdmin\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"unpause\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"currentAdmin\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"pendingAdmin\",\"type\":\"address\"}],\"name\":\"AdminTransferStarted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousAdmin\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newAdmin\",\"type\":\"address\"}],\"name\":\"AdminTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"ExecutorPaused\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"ExecutorUnpaused\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousPauser\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newPauser\",\"type\":\"address\"}],\"name\":\"PauserChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"baseToken\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"quoteToken\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"baseAmount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"quoteSpent\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"quoteReceived\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"quoteProfit\",\"type\":\"uint256\"}],\"name\":\"SpreadExecuted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"TokenRescued\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes4\",\"name\":\"selector\",\"type\":\"bytes4\"}],\"name\":\"CallNotAllowed\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"deadline\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"timestamp\",\"type\":\"uint256\"}],\"name\":\"DeadlineExpired\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"}],\"name\":\"DuplicateTarget\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"actualAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"requiredAmount\",\"type\":\"uint256\"}],\"name\":\"InsufficientBaseBought\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"actualAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"minimumAmount\",\"type\":\"uint256\"}],\"name\":\"InsufficientQuoteOutput\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"actualAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"minimumAmount\",\"type\":\"uint256\"}],\"name\":\"InsufficientQuoteProfit\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidAddress\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidAmount\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InvalidCallData\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"LengthMismatch\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"NotAdmin\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NotPaused\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"NotPauser\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"Paused\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ReentrantCall\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"}],\"name\":\"TokenCallFailed\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"},{\"internalType\":\"bytes\",\"name\":\"reason\",\"type\":\"bytes\"}],\"name\":\"VenueCallFailed\",\"type\":\"error\"}]",
}

// SpreadExecutorABI is the input ABI used to generate the binding from.
// Deprecated: Use SpreadExecutorMetaData.ABI instead.
var SpreadExecutorABI = SpreadExecutorMetaData.ABI

// SpreadExecutor is an auto generated Go binding around an Ethereum contract.
type SpreadExecutor struct {
	SpreadExecutorCaller     // Read-only binding to the contract
	SpreadExecutorTransactor // Write-only binding to the contract
	SpreadExecutorFilterer   // Log filterer for contract events
}

// SpreadExecutorCaller is an auto generated read-only Go binding around an Ethereum contract.
type SpreadExecutorCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SpreadExecutorTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SpreadExecutorTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SpreadExecutorFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SpreadExecutorFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SpreadExecutorSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SpreadExecutorSession struct {
	Contract     *SpreadExecutor   // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SpreadExecutorCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SpreadExecutorCallerSession struct {
	Contract *SpreadExecutorCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts         // Call options to use throughout this session
}

// SpreadExecutorTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SpreadExecutorTransactorSession struct {
	Contract     *SpreadExecutorTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts         // Transaction auth options to use throughout this session
}

// SpreadExecutorRaw is an auto generated low-level Go binding around an Ethereum contract.
type SpreadExecutorRaw struct {
	Contract *SpreadExecutor // Generic contract binding to access the raw methods on
}

// SpreadExecutorCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SpreadExecutorCallerRaw struct {
	Contract *SpreadExecutorCaller // Generic read-only contract binding to access the raw methods on
}

// SpreadExecutorTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SpreadExecutorTransactorRaw struct {
	Contract *SpreadExecutorTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSpreadExecutor creates a new instance of SpreadExecutor, bound to a specific deployed contract.
func NewSpreadExecutor(address common.Address, backend bind.ContractBackend) (*SpreadExecutor, error) {
	contract, err := bindSpreadExecutor(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutor{SpreadExecutorCaller: SpreadExecutorCaller{contract: contract}, SpreadExecutorTransactor: SpreadExecutorTransactor{contract: contract}, SpreadExecutorFilterer: SpreadExecutorFilterer{contract: contract}}, nil
}

// NewSpreadExecutorCaller creates a new read-only instance of SpreadExecutor, bound to a specific deployed contract.
func NewSpreadExecutorCaller(address common.Address, caller bind.ContractCaller) (*SpreadExecutorCaller, error) {
	contract, err := bindSpreadExecutor(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorCaller{contract: contract}, nil
}

// NewSpreadExecutorTransactor creates a new write-only instance of SpreadExecutor, bound to a specific deployed contract.
func NewSpreadExecutorTransactor(address common.Address, transactor bind.ContractTransactor) (*SpreadExecutorTransactor, error) {
	contract, err := bindSpreadExecutor(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorTransactor{contract: contract}, nil
}

// NewSpreadExecutorFilterer creates a new log filterer instance of SpreadExecutor, bound to a specific deployed contract.
func NewSpreadExecutorFilterer(address common.Address, filterer bind.ContractFilterer) (*SpreadExecutorFilterer, error) {
	contract, err := bindSpreadExecutor(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorFilterer{contract: contract}, nil
}

// bindSpreadExecutor binds a generic wrapper to an already deployed contract.
func bindSpreadExecutor(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SpreadExecutorMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SpreadExecutor *SpreadExecutorRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SpreadExecutor.Contract.SpreadExecutorCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SpreadExecutor *SpreadExecutorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.SpreadExecutorTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SpreadExecutor *SpreadExecutorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.SpreadExecutorTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SpreadExecutor *SpreadExecutorCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SpreadExecutor.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SpreadExecutor *SpreadExecutorTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SpreadExecutor *SpreadExecutorTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.contract.Transact(opts, method, params...)
}

// Admin is a free data retrieval call binding the contract method 0xf851a440.
//
// Solidity: function admin() view returns(address)
func (_SpreadExecutor *SpreadExecutorCaller) Admin(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _SpreadExecutor.contract.Call(opts, &out, "admin")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Admin is a free data retrieval call binding the contract method 0xf851a440.
//
// Solidity: function admin() view returns(address)
func (_SpreadExecutor *SpreadExecutorSession) Admin() (common.Address, error) {
	return _SpreadExecutor.Contract.Admin(&_SpreadExecutor.CallOpts)
}

// Admin is a free data retrieval call binding the contract method 0xf851a440.
//
// Solidity: function admin() view returns(address)
func (_SpreadExecutor *SpreadExecutorCallerSession) Admin() (common.Address, error) {
	return _SpreadExecutor.Contract.Admin(&_SpreadExecutor.CallOpts)
}

// IsCallAllowed is a free data retrieval call binding the contract method 0xac4fe96e.
//
// Solidity: function isCallAllowed(uint8 side, address target, bytes4 selector) view returns(bool)
func (_SpreadExecutor *SpreadExecutorCaller) IsCallAllowed(opts *bind.CallOpts, side uint8, target common.Address, selector [4]byte) (bool, error) {
	var out []interface{}
	err := _SpreadExecutor.contract.Call(opts, &out, "isCallAllowed", side, target, selector)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsCallAllowed is a free data retrieval call binding the contract method 0xac4fe96e.
//
// Solidity: function isCallAllowed(uint8 side, address target, bytes4 selector) view returns(bool)
func (_SpreadExecutor *SpreadExecutorSession) IsCallAllowed(side uint8, target common.Address, selector [4]byte) (bool, error) {
	return _SpreadExecutor.Contract.IsCallAllowed(&_SpreadExecutor.CallOpts, side, target, selector)
}

// IsCallAllowed is a free data retrieval call binding the contract method 0xac4fe96e.
//
// Solidity: function isCallAllowed(uint8 side, address target, bytes4 selector) view returns(bool)
func (_SpreadExecutor *SpreadExecutorCallerSession) IsCallAllowed(side uint8, target common.Address, selector [4]byte) (bool, error) {
	return _SpreadExecutor.Contract.IsCallAllowed(&_SpreadExecutor.CallOpts, side, target, selector)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_SpreadExecutor *SpreadExecutorCaller) Paused(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _SpreadExecutor.contract.Call(opts, &out, "paused")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_SpreadExecutor *SpreadExecutorSession) Paused() (bool, error) {
	return _SpreadExecutor.Contract.Paused(&_SpreadExecutor.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_SpreadExecutor *SpreadExecutorCallerSession) Paused() (bool, error) {
	return _SpreadExecutor.Contract.Paused(&_SpreadExecutor.CallOpts)
}

// Pauser is a free data retrieval call binding the contract method 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (_SpreadExecutor *SpreadExecutorCaller) Pauser(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _SpreadExecutor.contract.Call(opts, &out, "pauser")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Pauser is a free data retrieval call binding the contract method 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (_SpreadExecutor *SpreadExecutorSession) Pauser() (common.Address, error) {
	return _SpreadExecutor.Contract.Pauser(&_SpreadExecutor.CallOpts)
}

// Pauser is a free data retrieval call binding the contract method 0x9fd0506d.
//
// Solidity: function pauser() view returns(address)
func (_SpreadExecutor *SpreadExecutorCallerSession) Pauser() (common.Address, error) {
	return _SpreadExecutor.Contract.Pauser(&_SpreadExecutor.CallOpts)
}

// PendingAdmin is a free data retrieval call binding the contract method 0x26782247.
//
// Solidity: function pendingAdmin() view returns(address)
func (_SpreadExecutor *SpreadExecutorCaller) PendingAdmin(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _SpreadExecutor.contract.Call(opts, &out, "pendingAdmin")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PendingAdmin is a free data retrieval call binding the contract method 0x26782247.
//
// Solidity: function pendingAdmin() view returns(address)
func (_SpreadExecutor *SpreadExecutorSession) PendingAdmin() (common.Address, error) {
	return _SpreadExecutor.Contract.PendingAdmin(&_SpreadExecutor.CallOpts)
}

// PendingAdmin is a free data retrieval call binding the contract method 0x26782247.
//
// Solidity: function pendingAdmin() view returns(address)
func (_SpreadExecutor *SpreadExecutorCallerSession) PendingAdmin() (common.Address, error) {
	return _SpreadExecutor.Contract.PendingAdmin(&_SpreadExecutor.CallOpts)
}

// AcceptAdmin is a paid mutator transaction binding the contract method 0x0e18b681.
//
// Solidity: function acceptAdmin() returns()
func (_SpreadExecutor *SpreadExecutorTransactor) AcceptAdmin(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "acceptAdmin")
}

// AcceptAdmin is a paid mutator transaction binding the contract method 0x0e18b681.
//
// Solidity: function acceptAdmin() returns()
func (_SpreadExecutor *SpreadExecutorSession) AcceptAdmin() (*types.Transaction, error) {
	return _SpreadExecutor.Contract.AcceptAdmin(&_SpreadExecutor.TransactOpts)
}

// AcceptAdmin is a paid mutator transaction binding the contract method 0x0e18b681.
//
// Solidity: function acceptAdmin() returns()
func (_SpreadExecutor *SpreadExecutorTransactorSession) AcceptAdmin() (*types.Transaction, error) {
	return _SpreadExecutor.Contract.AcceptAdmin(&_SpreadExecutor.TransactOpts)
}

// ExecuteSpread is a paid mutator transaction binding the contract method 0x44914479.
//
// Solidity: function executeSpread((address,address,uint256,uint256,uint256,uint256,uint256,(address,bytes),(address,bytes)) execution) returns(uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit)
func (_SpreadExecutor *SpreadExecutorTransactor) ExecuteSpread(opts *bind.TransactOpts, execution ISpreadExecutorExecution) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "executeSpread", execution)
}

// ExecuteSpread is a paid mutator transaction binding the contract method 0x44914479.
//
// Solidity: function executeSpread((address,address,uint256,uint256,uint256,uint256,uint256,(address,bytes),(address,bytes)) execution) returns(uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit)
func (_SpreadExecutor *SpreadExecutorSession) ExecuteSpread(execution ISpreadExecutorExecution) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.ExecuteSpread(&_SpreadExecutor.TransactOpts, execution)
}

// ExecuteSpread is a paid mutator transaction binding the contract method 0x44914479.
//
// Solidity: function executeSpread((address,address,uint256,uint256,uint256,uint256,uint256,(address,bytes),(address,bytes)) execution) returns(uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit)
func (_SpreadExecutor *SpreadExecutorTransactorSession) ExecuteSpread(execution ISpreadExecutorExecution) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.ExecuteSpread(&_SpreadExecutor.TransactOpts, execution)
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_SpreadExecutor *SpreadExecutorTransactor) Pause(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "pause")
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_SpreadExecutor *SpreadExecutorSession) Pause() (*types.Transaction, error) {
	return _SpreadExecutor.Contract.Pause(&_SpreadExecutor.TransactOpts)
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_SpreadExecutor *SpreadExecutorTransactorSession) Pause() (*types.Transaction, error) {
	return _SpreadExecutor.Contract.Pause(&_SpreadExecutor.TransactOpts)
}

// RescueToken is a paid mutator transaction binding the contract method 0xe5711e8b.
//
// Solidity: function rescueToken(address token, address recipient, uint256 amount) returns()
func (_SpreadExecutor *SpreadExecutorTransactor) RescueToken(opts *bind.TransactOpts, token common.Address, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "rescueToken", token, recipient, amount)
}

// RescueToken is a paid mutator transaction binding the contract method 0xe5711e8b.
//
// Solidity: function rescueToken(address token, address recipient, uint256 amount) returns()
func (_SpreadExecutor *SpreadExecutorSession) RescueToken(token common.Address, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.RescueToken(&_SpreadExecutor.TransactOpts, token, recipient, amount)
}

// RescueToken is a paid mutator transaction binding the contract method 0xe5711e8b.
//
// Solidity: function rescueToken(address token, address recipient, uint256 amount) returns()
func (_SpreadExecutor *SpreadExecutorTransactorSession) RescueToken(token common.Address, recipient common.Address, amount *big.Int) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.RescueToken(&_SpreadExecutor.TransactOpts, token, recipient, amount)
}

// SetPauser is a paid mutator transaction binding the contract method 0x2d88af4a.
//
// Solidity: function setPauser(address newPauser) returns()
func (_SpreadExecutor *SpreadExecutorTransactor) SetPauser(opts *bind.TransactOpts, newPauser common.Address) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "setPauser", newPauser)
}

// SetPauser is a paid mutator transaction binding the contract method 0x2d88af4a.
//
// Solidity: function setPauser(address newPauser) returns()
func (_SpreadExecutor *SpreadExecutorSession) SetPauser(newPauser common.Address) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.SetPauser(&_SpreadExecutor.TransactOpts, newPauser)
}

// SetPauser is a paid mutator transaction binding the contract method 0x2d88af4a.
//
// Solidity: function setPauser(address newPauser) returns()
func (_SpreadExecutor *SpreadExecutorTransactorSession) SetPauser(newPauser common.Address) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.SetPauser(&_SpreadExecutor.TransactOpts, newPauser)
}

// TransferAdmin is a paid mutator transaction binding the contract method 0x75829def.
//
// Solidity: function transferAdmin(address newAdmin) returns()
func (_SpreadExecutor *SpreadExecutorTransactor) TransferAdmin(opts *bind.TransactOpts, newAdmin common.Address) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "transferAdmin", newAdmin)
}

// TransferAdmin is a paid mutator transaction binding the contract method 0x75829def.
//
// Solidity: function transferAdmin(address newAdmin) returns()
func (_SpreadExecutor *SpreadExecutorSession) TransferAdmin(newAdmin common.Address) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.TransferAdmin(&_SpreadExecutor.TransactOpts, newAdmin)
}

// TransferAdmin is a paid mutator transaction binding the contract method 0x75829def.
//
// Solidity: function transferAdmin(address newAdmin) returns()
func (_SpreadExecutor *SpreadExecutorTransactorSession) TransferAdmin(newAdmin common.Address) (*types.Transaction, error) {
	return _SpreadExecutor.Contract.TransferAdmin(&_SpreadExecutor.TransactOpts, newAdmin)
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_SpreadExecutor *SpreadExecutorTransactor) Unpause(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SpreadExecutor.contract.Transact(opts, "unpause")
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_SpreadExecutor *SpreadExecutorSession) Unpause() (*types.Transaction, error) {
	return _SpreadExecutor.Contract.Unpause(&_SpreadExecutor.TransactOpts)
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_SpreadExecutor *SpreadExecutorTransactorSession) Unpause() (*types.Transaction, error) {
	return _SpreadExecutor.Contract.Unpause(&_SpreadExecutor.TransactOpts)
}

// SpreadExecutorAdminTransferStartedIterator is returned from FilterAdminTransferStarted and is used to iterate over the raw logs and unpacked data for AdminTransferStarted events raised by the SpreadExecutor contract.
type SpreadExecutorAdminTransferStartedIterator struct {
	Event *SpreadExecutorAdminTransferStarted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorAdminTransferStartedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorAdminTransferStarted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorAdminTransferStarted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorAdminTransferStartedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorAdminTransferStartedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorAdminTransferStarted represents a AdminTransferStarted event raised by the SpreadExecutor contract.
type SpreadExecutorAdminTransferStarted struct {
	CurrentAdmin common.Address
	PendingAdmin common.Address
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterAdminTransferStarted is a free log retrieval operation binding the contract event 0xe5cd1c804f1c9cc6d7009e4c0fb532f0e2d8863524c3323a6b3790c3f80bf25c.
//
// Solidity: event AdminTransferStarted(address indexed currentAdmin, address indexed pendingAdmin)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterAdminTransferStarted(opts *bind.FilterOpts, currentAdmin []common.Address, pendingAdmin []common.Address) (*SpreadExecutorAdminTransferStartedIterator, error) {

	var currentAdminRule []interface{}
	for _, currentAdminItem := range currentAdmin {
		currentAdminRule = append(currentAdminRule, currentAdminItem)
	}
	var pendingAdminRule []interface{}
	for _, pendingAdminItem := range pendingAdmin {
		pendingAdminRule = append(pendingAdminRule, pendingAdminItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "AdminTransferStarted", currentAdminRule, pendingAdminRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorAdminTransferStartedIterator{contract: _SpreadExecutor.contract, event: "AdminTransferStarted", logs: logs, sub: sub}, nil
}

// WatchAdminTransferStarted is a free log subscription operation binding the contract event 0xe5cd1c804f1c9cc6d7009e4c0fb532f0e2d8863524c3323a6b3790c3f80bf25c.
//
// Solidity: event AdminTransferStarted(address indexed currentAdmin, address indexed pendingAdmin)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchAdminTransferStarted(opts *bind.WatchOpts, sink chan<- *SpreadExecutorAdminTransferStarted, currentAdmin []common.Address, pendingAdmin []common.Address) (event.Subscription, error) {

	var currentAdminRule []interface{}
	for _, currentAdminItem := range currentAdmin {
		currentAdminRule = append(currentAdminRule, currentAdminItem)
	}
	var pendingAdminRule []interface{}
	for _, pendingAdminItem := range pendingAdmin {
		pendingAdminRule = append(pendingAdminRule, pendingAdminItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "AdminTransferStarted", currentAdminRule, pendingAdminRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorAdminTransferStarted)
				if err := _SpreadExecutor.contract.UnpackLog(event, "AdminTransferStarted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAdminTransferStarted is a log parse operation binding the contract event 0xe5cd1c804f1c9cc6d7009e4c0fb532f0e2d8863524c3323a6b3790c3f80bf25c.
//
// Solidity: event AdminTransferStarted(address indexed currentAdmin, address indexed pendingAdmin)
func (_SpreadExecutor *SpreadExecutorFilterer) ParseAdminTransferStarted(log types.Log) (*SpreadExecutorAdminTransferStarted, error) {
	event := new(SpreadExecutorAdminTransferStarted)
	if err := _SpreadExecutor.contract.UnpackLog(event, "AdminTransferStarted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SpreadExecutorAdminTransferredIterator is returned from FilterAdminTransferred and is used to iterate over the raw logs and unpacked data for AdminTransferred events raised by the SpreadExecutor contract.
type SpreadExecutorAdminTransferredIterator struct {
	Event *SpreadExecutorAdminTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorAdminTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorAdminTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorAdminTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorAdminTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorAdminTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorAdminTransferred represents a AdminTransferred event raised by the SpreadExecutor contract.
type SpreadExecutorAdminTransferred struct {
	PreviousAdmin common.Address
	NewAdmin      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterAdminTransferred is a free log retrieval operation binding the contract event 0xf8ccb027dfcd135e000e9d45e6cc2d662578a8825d4c45b5e32e0adf67e79ec6.
//
// Solidity: event AdminTransferred(address indexed previousAdmin, address indexed newAdmin)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterAdminTransferred(opts *bind.FilterOpts, previousAdmin []common.Address, newAdmin []common.Address) (*SpreadExecutorAdminTransferredIterator, error) {

	var previousAdminRule []interface{}
	for _, previousAdminItem := range previousAdmin {
		previousAdminRule = append(previousAdminRule, previousAdminItem)
	}
	var newAdminRule []interface{}
	for _, newAdminItem := range newAdmin {
		newAdminRule = append(newAdminRule, newAdminItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "AdminTransferred", previousAdminRule, newAdminRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorAdminTransferredIterator{contract: _SpreadExecutor.contract, event: "AdminTransferred", logs: logs, sub: sub}, nil
}

// WatchAdminTransferred is a free log subscription operation binding the contract event 0xf8ccb027dfcd135e000e9d45e6cc2d662578a8825d4c45b5e32e0adf67e79ec6.
//
// Solidity: event AdminTransferred(address indexed previousAdmin, address indexed newAdmin)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchAdminTransferred(opts *bind.WatchOpts, sink chan<- *SpreadExecutorAdminTransferred, previousAdmin []common.Address, newAdmin []common.Address) (event.Subscription, error) {

	var previousAdminRule []interface{}
	for _, previousAdminItem := range previousAdmin {
		previousAdminRule = append(previousAdminRule, previousAdminItem)
	}
	var newAdminRule []interface{}
	for _, newAdminItem := range newAdmin {
		newAdminRule = append(newAdminRule, newAdminItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "AdminTransferred", previousAdminRule, newAdminRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorAdminTransferred)
				if err := _SpreadExecutor.contract.UnpackLog(event, "AdminTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseAdminTransferred is a log parse operation binding the contract event 0xf8ccb027dfcd135e000e9d45e6cc2d662578a8825d4c45b5e32e0adf67e79ec6.
//
// Solidity: event AdminTransferred(address indexed previousAdmin, address indexed newAdmin)
func (_SpreadExecutor *SpreadExecutorFilterer) ParseAdminTransferred(log types.Log) (*SpreadExecutorAdminTransferred, error) {
	event := new(SpreadExecutorAdminTransferred)
	if err := _SpreadExecutor.contract.UnpackLog(event, "AdminTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SpreadExecutorExecutorPausedIterator is returned from FilterExecutorPaused and is used to iterate over the raw logs and unpacked data for ExecutorPaused events raised by the SpreadExecutor contract.
type SpreadExecutorExecutorPausedIterator struct {
	Event *SpreadExecutorExecutorPaused // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorExecutorPausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorExecutorPaused)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorExecutorPaused)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorExecutorPausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorExecutorPausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorExecutorPaused represents a ExecutorPaused event raised by the SpreadExecutor contract.
type SpreadExecutorExecutorPaused struct {
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterExecutorPaused is a free log retrieval operation binding the contract event 0xb83b371003b5887345204dc6f6c1d604782d6130ef095176c2ce0d0a829707a4.
//
// Solidity: event ExecutorPaused(address indexed account)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterExecutorPaused(opts *bind.FilterOpts, account []common.Address) (*SpreadExecutorExecutorPausedIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "ExecutorPaused", accountRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorExecutorPausedIterator{contract: _SpreadExecutor.contract, event: "ExecutorPaused", logs: logs, sub: sub}, nil
}

// WatchExecutorPaused is a free log subscription operation binding the contract event 0xb83b371003b5887345204dc6f6c1d604782d6130ef095176c2ce0d0a829707a4.
//
// Solidity: event ExecutorPaused(address indexed account)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchExecutorPaused(opts *bind.WatchOpts, sink chan<- *SpreadExecutorExecutorPaused, account []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "ExecutorPaused", accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorExecutorPaused)
				if err := _SpreadExecutor.contract.UnpackLog(event, "ExecutorPaused", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseExecutorPaused is a log parse operation binding the contract event 0xb83b371003b5887345204dc6f6c1d604782d6130ef095176c2ce0d0a829707a4.
//
// Solidity: event ExecutorPaused(address indexed account)
func (_SpreadExecutor *SpreadExecutorFilterer) ParseExecutorPaused(log types.Log) (*SpreadExecutorExecutorPaused, error) {
	event := new(SpreadExecutorExecutorPaused)
	if err := _SpreadExecutor.contract.UnpackLog(event, "ExecutorPaused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SpreadExecutorExecutorUnpausedIterator is returned from FilterExecutorUnpaused and is used to iterate over the raw logs and unpacked data for ExecutorUnpaused events raised by the SpreadExecutor contract.
type SpreadExecutorExecutorUnpausedIterator struct {
	Event *SpreadExecutorExecutorUnpaused // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorExecutorUnpausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorExecutorUnpaused)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorExecutorUnpaused)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorExecutorUnpausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorExecutorUnpausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorExecutorUnpaused represents a ExecutorUnpaused event raised by the SpreadExecutor contract.
type SpreadExecutorExecutorUnpaused struct {
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterExecutorUnpaused is a free log retrieval operation binding the contract event 0x3d9c271ab014260bbb38be1ca4f9b5d967b72b9e3942858c0b319bf39278607c.
//
// Solidity: event ExecutorUnpaused(address indexed account)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterExecutorUnpaused(opts *bind.FilterOpts, account []common.Address) (*SpreadExecutorExecutorUnpausedIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "ExecutorUnpaused", accountRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorExecutorUnpausedIterator{contract: _SpreadExecutor.contract, event: "ExecutorUnpaused", logs: logs, sub: sub}, nil
}

// WatchExecutorUnpaused is a free log subscription operation binding the contract event 0x3d9c271ab014260bbb38be1ca4f9b5d967b72b9e3942858c0b319bf39278607c.
//
// Solidity: event ExecutorUnpaused(address indexed account)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchExecutorUnpaused(opts *bind.WatchOpts, sink chan<- *SpreadExecutorExecutorUnpaused, account []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "ExecutorUnpaused", accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorExecutorUnpaused)
				if err := _SpreadExecutor.contract.UnpackLog(event, "ExecutorUnpaused", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseExecutorUnpaused is a log parse operation binding the contract event 0x3d9c271ab014260bbb38be1ca4f9b5d967b72b9e3942858c0b319bf39278607c.
//
// Solidity: event ExecutorUnpaused(address indexed account)
func (_SpreadExecutor *SpreadExecutorFilterer) ParseExecutorUnpaused(log types.Log) (*SpreadExecutorExecutorUnpaused, error) {
	event := new(SpreadExecutorExecutorUnpaused)
	if err := _SpreadExecutor.contract.UnpackLog(event, "ExecutorUnpaused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SpreadExecutorPauserChangedIterator is returned from FilterPauserChanged and is used to iterate over the raw logs and unpacked data for PauserChanged events raised by the SpreadExecutor contract.
type SpreadExecutorPauserChangedIterator struct {
	Event *SpreadExecutorPauserChanged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorPauserChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorPauserChanged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorPauserChanged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorPauserChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorPauserChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorPauserChanged represents a PauserChanged event raised by the SpreadExecutor contract.
type SpreadExecutorPauserChanged struct {
	PreviousPauser common.Address
	NewPauser      common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterPauserChanged is a free log retrieval operation binding the contract event 0x95bb211a5a393c4d30c3edc9a745825fba4e6ad3e3bb949e6bf8ccdfe431a811.
//
// Solidity: event PauserChanged(address indexed previousPauser, address indexed newPauser)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterPauserChanged(opts *bind.FilterOpts, previousPauser []common.Address, newPauser []common.Address) (*SpreadExecutorPauserChangedIterator, error) {

	var previousPauserRule []interface{}
	for _, previousPauserItem := range previousPauser {
		previousPauserRule = append(previousPauserRule, previousPauserItem)
	}
	var newPauserRule []interface{}
	for _, newPauserItem := range newPauser {
		newPauserRule = append(newPauserRule, newPauserItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "PauserChanged", previousPauserRule, newPauserRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorPauserChangedIterator{contract: _SpreadExecutor.contract, event: "PauserChanged", logs: logs, sub: sub}, nil
}

// WatchPauserChanged is a free log subscription operation binding the contract event 0x95bb211a5a393c4d30c3edc9a745825fba4e6ad3e3bb949e6bf8ccdfe431a811.
//
// Solidity: event PauserChanged(address indexed previousPauser, address indexed newPauser)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchPauserChanged(opts *bind.WatchOpts, sink chan<- *SpreadExecutorPauserChanged, previousPauser []common.Address, newPauser []common.Address) (event.Subscription, error) {

	var previousPauserRule []interface{}
	for _, previousPauserItem := range previousPauser {
		previousPauserRule = append(previousPauserRule, previousPauserItem)
	}
	var newPauserRule []interface{}
	for _, newPauserItem := range newPauser {
		newPauserRule = append(newPauserRule, newPauserItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "PauserChanged", previousPauserRule, newPauserRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorPauserChanged)
				if err := _SpreadExecutor.contract.UnpackLog(event, "PauserChanged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParsePauserChanged is a log parse operation binding the contract event 0x95bb211a5a393c4d30c3edc9a745825fba4e6ad3e3bb949e6bf8ccdfe431a811.
//
// Solidity: event PauserChanged(address indexed previousPauser, address indexed newPauser)
func (_SpreadExecutor *SpreadExecutorFilterer) ParsePauserChanged(log types.Log) (*SpreadExecutorPauserChanged, error) {
	event := new(SpreadExecutorPauserChanged)
	if err := _SpreadExecutor.contract.UnpackLog(event, "PauserChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SpreadExecutorSpreadExecutedIterator is returned from FilterSpreadExecuted and is used to iterate over the raw logs and unpacked data for SpreadExecuted events raised by the SpreadExecutor contract.
type SpreadExecutorSpreadExecutedIterator struct {
	Event *SpreadExecutorSpreadExecuted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorSpreadExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorSpreadExecuted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorSpreadExecuted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorSpreadExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorSpreadExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorSpreadExecuted represents a SpreadExecuted event raised by the SpreadExecutor contract.
type SpreadExecutorSpreadExecuted struct {
	Account       common.Address
	BaseToken     common.Address
	QuoteToken    common.Address
	BaseAmount    *big.Int
	QuoteSpent    *big.Int
	QuoteReceived *big.Int
	QuoteProfit   *big.Int
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterSpreadExecuted is a free log retrieval operation binding the contract event 0x8bac7fc505299c7681fb3299071455533bc3ed66a371a8998d7b5e15ac10bac2.
//
// Solidity: event SpreadExecuted(address indexed account, address indexed baseToken, address indexed quoteToken, uint256 baseAmount, uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterSpreadExecuted(opts *bind.FilterOpts, account []common.Address, baseToken []common.Address, quoteToken []common.Address) (*SpreadExecutorSpreadExecutedIterator, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var baseTokenRule []interface{}
	for _, baseTokenItem := range baseToken {
		baseTokenRule = append(baseTokenRule, baseTokenItem)
	}
	var quoteTokenRule []interface{}
	for _, quoteTokenItem := range quoteToken {
		quoteTokenRule = append(quoteTokenRule, quoteTokenItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "SpreadExecuted", accountRule, baseTokenRule, quoteTokenRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorSpreadExecutedIterator{contract: _SpreadExecutor.contract, event: "SpreadExecuted", logs: logs, sub: sub}, nil
}

// WatchSpreadExecuted is a free log subscription operation binding the contract event 0x8bac7fc505299c7681fb3299071455533bc3ed66a371a8998d7b5e15ac10bac2.
//
// Solidity: event SpreadExecuted(address indexed account, address indexed baseToken, address indexed quoteToken, uint256 baseAmount, uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchSpreadExecuted(opts *bind.WatchOpts, sink chan<- *SpreadExecutorSpreadExecuted, account []common.Address, baseToken []common.Address, quoteToken []common.Address) (event.Subscription, error) {

	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var baseTokenRule []interface{}
	for _, baseTokenItem := range baseToken {
		baseTokenRule = append(baseTokenRule, baseTokenItem)
	}
	var quoteTokenRule []interface{}
	for _, quoteTokenItem := range quoteToken {
		quoteTokenRule = append(quoteTokenRule, quoteTokenItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "SpreadExecuted", accountRule, baseTokenRule, quoteTokenRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorSpreadExecuted)
				if err := _SpreadExecutor.contract.UnpackLog(event, "SpreadExecuted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseSpreadExecuted is a log parse operation binding the contract event 0x8bac7fc505299c7681fb3299071455533bc3ed66a371a8998d7b5e15ac10bac2.
//
// Solidity: event SpreadExecuted(address indexed account, address indexed baseToken, address indexed quoteToken, uint256 baseAmount, uint256 quoteSpent, uint256 quoteReceived, uint256 quoteProfit)
func (_SpreadExecutor *SpreadExecutorFilterer) ParseSpreadExecuted(log types.Log) (*SpreadExecutorSpreadExecuted, error) {
	event := new(SpreadExecutorSpreadExecuted)
	if err := _SpreadExecutor.contract.UnpackLog(event, "SpreadExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SpreadExecutorTokenRescuedIterator is returned from FilterTokenRescued and is used to iterate over the raw logs and unpacked data for TokenRescued events raised by the SpreadExecutor contract.
type SpreadExecutorTokenRescuedIterator struct {
	Event *SpreadExecutorTokenRescued // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SpreadExecutorTokenRescuedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SpreadExecutorTokenRescued)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SpreadExecutorTokenRescued)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SpreadExecutorTokenRescuedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SpreadExecutorTokenRescuedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SpreadExecutorTokenRescued represents a TokenRescued event raised by the SpreadExecutor contract.
type SpreadExecutorTokenRescued struct {
	Token     common.Address
	Recipient common.Address
	Amount    *big.Int
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterTokenRescued is a free log retrieval operation binding the contract event 0x4143f7b5cb6ea007914c32b8a3e64cebc051d7f493fa0755454da1e47701e125.
//
// Solidity: event TokenRescued(address indexed token, address indexed recipient, uint256 amount)
func (_SpreadExecutor *SpreadExecutorFilterer) FilterTokenRescued(opts *bind.FilterOpts, token []common.Address, recipient []common.Address) (*SpreadExecutorTokenRescuedIterator, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	logs, sub, err := _SpreadExecutor.contract.FilterLogs(opts, "TokenRescued", tokenRule, recipientRule)
	if err != nil {
		return nil, err
	}
	return &SpreadExecutorTokenRescuedIterator{contract: _SpreadExecutor.contract, event: "TokenRescued", logs: logs, sub: sub}, nil
}

// WatchTokenRescued is a free log subscription operation binding the contract event 0x4143f7b5cb6ea007914c32b8a3e64cebc051d7f493fa0755454da1e47701e125.
//
// Solidity: event TokenRescued(address indexed token, address indexed recipient, uint256 amount)
func (_SpreadExecutor *SpreadExecutorFilterer) WatchTokenRescued(opts *bind.WatchOpts, sink chan<- *SpreadExecutorTokenRescued, token []common.Address, recipient []common.Address) (event.Subscription, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	logs, sub, err := _SpreadExecutor.contract.WatchLogs(opts, "TokenRescued", tokenRule, recipientRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SpreadExecutorTokenRescued)
				if err := _SpreadExecutor.contract.UnpackLog(event, "TokenRescued", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseTokenRescued is a log parse operation binding the contract event 0x4143f7b5cb6ea007914c32b8a3e64cebc051d7f493fa0755454da1e47701e125.
//
// Solidity: event TokenRescued(address indexed token, address indexed recipient, uint256 amount)
func (_SpreadExecutor *SpreadExecutorFilterer) ParseTokenRescued(log types.Log) (*SpreadExecutorTokenRescued, error) {
	event := new(SpreadExecutorTokenRescued)
	if err := _SpreadExecutor.contract.UnpackLog(event, "TokenRescued", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
