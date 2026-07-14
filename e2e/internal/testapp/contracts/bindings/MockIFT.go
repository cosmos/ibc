// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package bindings

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

// MockIFTMetaData contains all meta data concerning the MockIFT contract.
var MockIFTMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"balanceOf\",\"inputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"mint\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"receiveTransfer\",\"inputs\":[{\"name\":\"routeId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"s\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"refund\",\"inputs\":[{\"name\":\"s\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"sendTransfer\",\"inputs\":[{\"name\":\"routeId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"receiver\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"timeoutTimestamp\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"IFTReceived\",\"inputs\":[{\"name\":\"routeId\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"seq\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"receiver\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"IFTRefunded\",\"inputs\":[{\"name\":\"seq\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"sender\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"IFTSent\",\"inputs\":[{\"name\":\"seq\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"routeId\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"receiver\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"timeoutTimestamp\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false}]",
	Bin: "0x6080604052348015600e575f5ffd5b506107048061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610055575f3560e01c8063278ecde11461005957806340c10f191461006e57806358d46e891461008157806370a082311461009457806389185769146100c5575b5f5ffd5b61006c610067366004610436565b6100d8565b005b61006c61007c366004610468565b610247565b61006c61008f3660046104d5565b610277565b6100b36100a2366004610535565b5f6020819052908152604090205481565b60405190815260200160405180910390f35b61006c6100d3366004610555565b6102ea565b5f81815260026020526040902060018101546101275760405162461bcd60e51b81526020600482015260096024820152686e6f20657363726f7760b81b60448201526064015b60405180910390fd5b80600201545f0361016b5760405162461bcd60e51b815260206004820152600e60248201526d1b9bc81d1a5b595bdd5d081cd95d60921b604482015260640161011e565b600381015460ff16156101b35760405162461bcd60e51b815260206004820152601060248201526f185b1c9958591e481c99599d5b99195960821b604482015260640161011e565b60038101805460ff1916600190811790915581015481546001600160a01b03165f90815260208190526040812080549091906101f09084906105e6565b909155505080546001820154604080518581526001600160a01b0390931660208401528201527f56987a7c1f738a2628ad3a97d8dd0d3d39cf208b5b95f2f8488c924ee7a857f09060600160405180910390a15050565b6001600160a01b0382165f908152602081905260408120805483929061026e9084906105e6565b90915550505050565b6001600160a01b0382165f908152602081905260408120805483929061029e9084906105e6565b90915550506040517fb5df872fe8db758a69d0f8500566a823f40dc55b858395ae89a58689ed51cfdb906102db9087908790879087908790610627565b60405180910390a15050505050565b335f9081526020819052604090205482111561033f5760405162461bcd60e51b8152602060048201526014602482015273696e73756666696369656e742062616c616e636560601b604482015260640161011e565b335f908152602081905260408120805484929061035d908490610660565b925050819055505f60015f815461037390610673565b91829055506040805160808101825233815260208082018781528284018781525f6060850181815287825260029485905290869020945185546001600160a01b0319166001600160a01b03909116178555915160018501555191830191909155516003909101805460ff1916911515919091179055519091507fa46e3879c3d5957518251cfe90f7589d3e38c7ebb4f4ac2b46bf8d58fcbb3709906104259083908a908a908a908a908a908a9061068b565b60405180910390a150505050505050565b5f60208284031215610446575f5ffd5b5035919050565b80356001600160a01b0381168114610463575f5ffd5b919050565b5f5f60408385031215610479575f5ffd5b6104828361044d565b946020939093013593505050565b5f5f83601f8401126104a0575f5ffd5b50813567ffffffffffffffff8111156104b7575f5ffd5b6020830191508360208285010111156104ce575f5ffd5b9250929050565b5f5f5f5f5f608086880312156104e9575f5ffd5b853567ffffffffffffffff8111156104ff575f5ffd5b61050b88828901610490565b909650945050602086013592506105246040870161044d565b949793965091946060013592915050565b5f60208284031215610545575f5ffd5b61054e8261044d565b9392505050565b5f5f5f5f5f5f6080878903121561056a575f5ffd5b863567ffffffffffffffff811115610580575f5ffd5b61058c89828a01610490565b909750955050602087013567ffffffffffffffff8111156105ab575f5ffd5b6105b789828a01610490565b979a9699509760408101359660609091013595509350505050565b634e487b7160e01b5f52601160045260245ffd5b808201808211156105f9576105f96105d2565b92915050565b81835281816020850137505f828201602090810191909152601f909101601f19169091010190565b608081525f61063a6080830187896105ff565b6020830195909552506001600160a01b0392909216604083015260609091015292915050565b818103818111156105f9576105f96105d2565b5f60018201610684576106846105d2565b5060010190565b87815260a060208201525f6106a460a08301888a6105ff565b82810360408401526106b78187896105ff565b60608401959095525050608001529594505050505056fea2646970667358221220cb391dc343b421d918e2e47016eb69321dbcfdf0dc885e069797f5f3d350728964736f6c634300081c0033",
}

// MockIFTABI is the input ABI used to generate the binding from.
// Deprecated: Use MockIFTMetaData.ABI instead.
var MockIFTABI = MockIFTMetaData.ABI

// MockIFTBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use MockIFTMetaData.Bin instead.
var MockIFTBin = MockIFTMetaData.Bin

// DeployMockIFT deploys a new Ethereum contract, binding an instance of MockIFT to it.
func DeployMockIFT(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *MockIFT, error) {
	parsed, err := MockIFTMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(MockIFTBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &MockIFT{MockIFTCaller: MockIFTCaller{contract: contract}, MockIFTTransactor: MockIFTTransactor{contract: contract}, MockIFTFilterer: MockIFTFilterer{contract: contract}}, nil
}

// MockIFT is an auto generated Go binding around an Ethereum contract.
type MockIFT struct {
	MockIFTCaller     // Read-only binding to the contract
	MockIFTTransactor // Write-only binding to the contract
	MockIFTFilterer   // Log filterer for contract events
}

// MockIFTCaller is an auto generated read-only Go binding around an Ethereum contract.
type MockIFTCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MockIFTTransactor is an auto generated write-only Go binding around an Ethereum contract.
type MockIFTTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MockIFTFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type MockIFTFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MockIFTSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type MockIFTSession struct {
	Contract     *MockIFT          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MockIFTCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type MockIFTCallerSession struct {
	Contract *MockIFTCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// MockIFTTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type MockIFTTransactorSession struct {
	Contract     *MockIFTTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// MockIFTRaw is an auto generated low-level Go binding around an Ethereum contract.
type MockIFTRaw struct {
	Contract *MockIFT // Generic contract binding to access the raw methods on
}

// MockIFTCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type MockIFTCallerRaw struct {
	Contract *MockIFTCaller // Generic read-only contract binding to access the raw methods on
}

// MockIFTTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type MockIFTTransactorRaw struct {
	Contract *MockIFTTransactor // Generic write-only contract binding to access the raw methods on
}

// NewMockIFT creates a new instance of MockIFT, bound to a specific deployed contract.
func NewMockIFT(address common.Address, backend bind.ContractBackend) (*MockIFT, error) {
	contract, err := bindMockIFT(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &MockIFT{MockIFTCaller: MockIFTCaller{contract: contract}, MockIFTTransactor: MockIFTTransactor{contract: contract}, MockIFTFilterer: MockIFTFilterer{contract: contract}}, nil
}

// NewMockIFTCaller creates a new read-only instance of MockIFT, bound to a specific deployed contract.
func NewMockIFTCaller(address common.Address, caller bind.ContractCaller) (*MockIFTCaller, error) {
	contract, err := bindMockIFT(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &MockIFTCaller{contract: contract}, nil
}

// NewMockIFTTransactor creates a new write-only instance of MockIFT, bound to a specific deployed contract.
func NewMockIFTTransactor(address common.Address, transactor bind.ContractTransactor) (*MockIFTTransactor, error) {
	contract, err := bindMockIFT(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &MockIFTTransactor{contract: contract}, nil
}

// NewMockIFTFilterer creates a new log filterer instance of MockIFT, bound to a specific deployed contract.
func NewMockIFTFilterer(address common.Address, filterer bind.ContractFilterer) (*MockIFTFilterer, error) {
	contract, err := bindMockIFT(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &MockIFTFilterer{contract: contract}, nil
}

// bindMockIFT binds a generic wrapper to an already deployed contract.
func bindMockIFT(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := MockIFTMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_MockIFT *MockIFTRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _MockIFT.Contract.MockIFTCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_MockIFT *MockIFTRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _MockIFT.Contract.MockIFTTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_MockIFT *MockIFTRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _MockIFT.Contract.MockIFTTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_MockIFT *MockIFTCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _MockIFT.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_MockIFT *MockIFTTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _MockIFT.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_MockIFT *MockIFTTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _MockIFT.Contract.contract.Transact(opts, method, params...)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_MockIFT *MockIFTCaller) BalanceOf(opts *bind.CallOpts, arg0 common.Address) (*big.Int, error) {
	var out []interface{}
	err := _MockIFT.contract.Call(opts, &out, "balanceOf", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_MockIFT *MockIFTSession) BalanceOf(arg0 common.Address) (*big.Int, error) {
	return _MockIFT.Contract.BalanceOf(&_MockIFT.CallOpts, arg0)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address ) view returns(uint256)
func (_MockIFT *MockIFTCallerSession) BalanceOf(arg0 common.Address) (*big.Int, error) {
	return _MockIFT.Contract.BalanceOf(&_MockIFT.CallOpts, arg0)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_MockIFT *MockIFTTransactor) Mint(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _MockIFT.contract.Transact(opts, "mint", to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_MockIFT *MockIFTSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.Mint(&_MockIFT.TransactOpts, to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_MockIFT *MockIFTTransactorSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.Mint(&_MockIFT.TransactOpts, to, amount)
}

// ReceiveTransfer is a paid mutator transaction binding the contract method 0x58d46e89.
//
// Solidity: function receiveTransfer(string routeId, uint256 s, address receiver, uint256 amount) returns()
func (_MockIFT *MockIFTTransactor) ReceiveTransfer(opts *bind.TransactOpts, routeId string, s *big.Int, receiver common.Address, amount *big.Int) (*types.Transaction, error) {
	return _MockIFT.contract.Transact(opts, "receiveTransfer", routeId, s, receiver, amount)
}

// ReceiveTransfer is a paid mutator transaction binding the contract method 0x58d46e89.
//
// Solidity: function receiveTransfer(string routeId, uint256 s, address receiver, uint256 amount) returns()
func (_MockIFT *MockIFTSession) ReceiveTransfer(routeId string, s *big.Int, receiver common.Address, amount *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.ReceiveTransfer(&_MockIFT.TransactOpts, routeId, s, receiver, amount)
}

// ReceiveTransfer is a paid mutator transaction binding the contract method 0x58d46e89.
//
// Solidity: function receiveTransfer(string routeId, uint256 s, address receiver, uint256 amount) returns()
func (_MockIFT *MockIFTTransactorSession) ReceiveTransfer(routeId string, s *big.Int, receiver common.Address, amount *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.ReceiveTransfer(&_MockIFT.TransactOpts, routeId, s, receiver, amount)
}

// Refund is a paid mutator transaction binding the contract method 0x278ecde1.
//
// Solidity: function refund(uint256 s) returns()
func (_MockIFT *MockIFTTransactor) Refund(opts *bind.TransactOpts, s *big.Int) (*types.Transaction, error) {
	return _MockIFT.contract.Transact(opts, "refund", s)
}

// Refund is a paid mutator transaction binding the contract method 0x278ecde1.
//
// Solidity: function refund(uint256 s) returns()
func (_MockIFT *MockIFTSession) Refund(s *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.Refund(&_MockIFT.TransactOpts, s)
}

// Refund is a paid mutator transaction binding the contract method 0x278ecde1.
//
// Solidity: function refund(uint256 s) returns()
func (_MockIFT *MockIFTTransactorSession) Refund(s *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.Refund(&_MockIFT.TransactOpts, s)
}

// SendTransfer is a paid mutator transaction binding the contract method 0x89185769.
//
// Solidity: function sendTransfer(string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp) returns()
func (_MockIFT *MockIFTTransactor) SendTransfer(opts *bind.TransactOpts, routeId string, receiver string, amount *big.Int, timeoutTimestamp *big.Int) (*types.Transaction, error) {
	return _MockIFT.contract.Transact(opts, "sendTransfer", routeId, receiver, amount, timeoutTimestamp)
}

// SendTransfer is a paid mutator transaction binding the contract method 0x89185769.
//
// Solidity: function sendTransfer(string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp) returns()
func (_MockIFT *MockIFTSession) SendTransfer(routeId string, receiver string, amount *big.Int, timeoutTimestamp *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.SendTransfer(&_MockIFT.TransactOpts, routeId, receiver, amount, timeoutTimestamp)
}

// SendTransfer is a paid mutator transaction binding the contract method 0x89185769.
//
// Solidity: function sendTransfer(string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp) returns()
func (_MockIFT *MockIFTTransactorSession) SendTransfer(routeId string, receiver string, amount *big.Int, timeoutTimestamp *big.Int) (*types.Transaction, error) {
	return _MockIFT.Contract.SendTransfer(&_MockIFT.TransactOpts, routeId, receiver, amount, timeoutTimestamp)
}

// MockIFTIFTReceivedIterator is returned from FilterIFTReceived and is used to iterate over the raw logs and unpacked data for IFTReceived events raised by the MockIFT contract.
type MockIFTIFTReceivedIterator struct {
	Event *MockIFTIFTReceived // Event containing the contract specifics and raw log

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
func (it *MockIFTIFTReceivedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MockIFTIFTReceived)
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
		it.Event = new(MockIFTIFTReceived)
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
func (it *MockIFTIFTReceivedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MockIFTIFTReceivedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MockIFTIFTReceived represents a IFTReceived event raised by the MockIFT contract.
type MockIFTIFTReceived struct {
	RouteId  string
	Seq      *big.Int
	Receiver common.Address
	Amount   *big.Int
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterIFTReceived is a free log retrieval operation binding the contract event 0xb5df872fe8db758a69d0f8500566a823f40dc55b858395ae89a58689ed51cfdb.
//
// Solidity: event IFTReceived(string routeId, uint256 seq, address receiver, uint256 amount)
func (_MockIFT *MockIFTFilterer) FilterIFTReceived(opts *bind.FilterOpts) (*MockIFTIFTReceivedIterator, error) {

	logs, sub, err := _MockIFT.contract.FilterLogs(opts, "IFTReceived")
	if err != nil {
		return nil, err
	}
	return &MockIFTIFTReceivedIterator{contract: _MockIFT.contract, event: "IFTReceived", logs: logs, sub: sub}, nil
}

// WatchIFTReceived is a free log subscription operation binding the contract event 0xb5df872fe8db758a69d0f8500566a823f40dc55b858395ae89a58689ed51cfdb.
//
// Solidity: event IFTReceived(string routeId, uint256 seq, address receiver, uint256 amount)
func (_MockIFT *MockIFTFilterer) WatchIFTReceived(opts *bind.WatchOpts, sink chan<- *MockIFTIFTReceived) (event.Subscription, error) {

	logs, sub, err := _MockIFT.contract.WatchLogs(opts, "IFTReceived")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MockIFTIFTReceived)
				if err := _MockIFT.contract.UnpackLog(event, "IFTReceived", log); err != nil {
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

// ParseIFTReceived is a log parse operation binding the contract event 0xb5df872fe8db758a69d0f8500566a823f40dc55b858395ae89a58689ed51cfdb.
//
// Solidity: event IFTReceived(string routeId, uint256 seq, address receiver, uint256 amount)
func (_MockIFT *MockIFTFilterer) ParseIFTReceived(log types.Log) (*MockIFTIFTReceived, error) {
	event := new(MockIFTIFTReceived)
	if err := _MockIFT.contract.UnpackLog(event, "IFTReceived", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// MockIFTIFTRefundedIterator is returned from FilterIFTRefunded and is used to iterate over the raw logs and unpacked data for IFTRefunded events raised by the MockIFT contract.
type MockIFTIFTRefundedIterator struct {
	Event *MockIFTIFTRefunded // Event containing the contract specifics and raw log

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
func (it *MockIFTIFTRefundedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MockIFTIFTRefunded)
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
		it.Event = new(MockIFTIFTRefunded)
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
func (it *MockIFTIFTRefundedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MockIFTIFTRefundedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MockIFTIFTRefunded represents a IFTRefunded event raised by the MockIFT contract.
type MockIFTIFTRefunded struct {
	Seq    *big.Int
	Sender common.Address
	Amount *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterIFTRefunded is a free log retrieval operation binding the contract event 0x56987a7c1f738a2628ad3a97d8dd0d3d39cf208b5b95f2f8488c924ee7a857f0.
//
// Solidity: event IFTRefunded(uint256 seq, address sender, uint256 amount)
func (_MockIFT *MockIFTFilterer) FilterIFTRefunded(opts *bind.FilterOpts) (*MockIFTIFTRefundedIterator, error) {

	logs, sub, err := _MockIFT.contract.FilterLogs(opts, "IFTRefunded")
	if err != nil {
		return nil, err
	}
	return &MockIFTIFTRefundedIterator{contract: _MockIFT.contract, event: "IFTRefunded", logs: logs, sub: sub}, nil
}

// WatchIFTRefunded is a free log subscription operation binding the contract event 0x56987a7c1f738a2628ad3a97d8dd0d3d39cf208b5b95f2f8488c924ee7a857f0.
//
// Solidity: event IFTRefunded(uint256 seq, address sender, uint256 amount)
func (_MockIFT *MockIFTFilterer) WatchIFTRefunded(opts *bind.WatchOpts, sink chan<- *MockIFTIFTRefunded) (event.Subscription, error) {

	logs, sub, err := _MockIFT.contract.WatchLogs(opts, "IFTRefunded")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MockIFTIFTRefunded)
				if err := _MockIFT.contract.UnpackLog(event, "IFTRefunded", log); err != nil {
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

// ParseIFTRefunded is a log parse operation binding the contract event 0x56987a7c1f738a2628ad3a97d8dd0d3d39cf208b5b95f2f8488c924ee7a857f0.
//
// Solidity: event IFTRefunded(uint256 seq, address sender, uint256 amount)
func (_MockIFT *MockIFTFilterer) ParseIFTRefunded(log types.Log) (*MockIFTIFTRefunded, error) {
	event := new(MockIFTIFTRefunded)
	if err := _MockIFT.contract.UnpackLog(event, "IFTRefunded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// MockIFTIFTSentIterator is returned from FilterIFTSent and is used to iterate over the raw logs and unpacked data for IFTSent events raised by the MockIFT contract.
type MockIFTIFTSentIterator struct {
	Event *MockIFTIFTSent // Event containing the contract specifics and raw log

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
func (it *MockIFTIFTSentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MockIFTIFTSent)
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
		it.Event = new(MockIFTIFTSent)
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
func (it *MockIFTIFTSentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MockIFTIFTSentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MockIFTIFTSent represents a IFTSent event raised by the MockIFT contract.
type MockIFTIFTSent struct {
	Seq              *big.Int
	RouteId          string
	Receiver         string
	Amount           *big.Int
	TimeoutTimestamp *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterIFTSent is a free log retrieval operation binding the contract event 0xa46e3879c3d5957518251cfe90f7589d3e38c7ebb4f4ac2b46bf8d58fcbb3709.
//
// Solidity: event IFTSent(uint256 seq, string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp)
func (_MockIFT *MockIFTFilterer) FilterIFTSent(opts *bind.FilterOpts) (*MockIFTIFTSentIterator, error) {

	logs, sub, err := _MockIFT.contract.FilterLogs(opts, "IFTSent")
	if err != nil {
		return nil, err
	}
	return &MockIFTIFTSentIterator{contract: _MockIFT.contract, event: "IFTSent", logs: logs, sub: sub}, nil
}

// WatchIFTSent is a free log subscription operation binding the contract event 0xa46e3879c3d5957518251cfe90f7589d3e38c7ebb4f4ac2b46bf8d58fcbb3709.
//
// Solidity: event IFTSent(uint256 seq, string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp)
func (_MockIFT *MockIFTFilterer) WatchIFTSent(opts *bind.WatchOpts, sink chan<- *MockIFTIFTSent) (event.Subscription, error) {

	logs, sub, err := _MockIFT.contract.WatchLogs(opts, "IFTSent")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MockIFTIFTSent)
				if err := _MockIFT.contract.UnpackLog(event, "IFTSent", log); err != nil {
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

// ParseIFTSent is a log parse operation binding the contract event 0xa46e3879c3d5957518251cfe90f7589d3e38c7ebb4f4ac2b46bf8d58fcbb3709.
//
// Solidity: event IFTSent(uint256 seq, string routeId, string receiver, uint256 amount, uint256 timeoutTimestamp)
func (_MockIFT *MockIFTFilterer) ParseIFTSent(log types.Log) (*MockIFTIFTSent, error) {
	event := new(MockIFTIFTSent)
	if err := _MockIFT.contract.UnpackLog(event, "IFTSent", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
