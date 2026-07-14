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

// MockGMPMetaData contains all meta data concerning the MockGMP contract.
var MockGMPMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"deliver\",\"inputs\":[{\"name\":\"routeId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"s\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payload\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"send\",\"inputs\":[{\"name\":\"routeId\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"target\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"payload\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"GMPReceived\",\"inputs\":[{\"name\":\"routeId\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"seq\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"target\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"success\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"GMPSent\",\"inputs\":[{\"name\":\"seq\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"routeId\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"target\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"payload\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false}]",
	Bin: "0x6080604052348015600e575f5ffd5b506103cd8061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610034575f3560e01c80631e0d43b914610038578063b6427b9c1461004d575b5f5ffd5b61004b61004636600461019b565b610060565b005b61004b61005b36600461023a565b6100c2565b5f5f5f815461006e906102b2565b91905081905590507f0ffa85d772b952085fe1134bf99ab2e93f970b4f6ee17d4304a04a03cdffb04f818888888888886040516100b197969594939291906102fe565b60405180910390a150505050505050565b5f836001600160a01b031683836040516100dd92919061034d565b5f604051808303815f865af19150503d805f8114610116576040519150601f19603f3d011682016040523d82523d5f602084013e61011b565b606091505b505090507fd7fdb4bb9c364fe7b30d3fe638a8b3f3bae426dee14cab821f4297b17cb6fbd987878787856040516100b195949392919061035c565b5f5f83601f840112610166575f5ffd5b50813567ffffffffffffffff81111561017d575f5ffd5b602083019150836020828501011115610194575f5ffd5b9250929050565b5f5f5f5f5f5f606087890312156101b0575f5ffd5b863567ffffffffffffffff8111156101c6575f5ffd5b6101d289828a01610156565b909750955050602087013567ffffffffffffffff8111156101f1575f5ffd5b6101fd89828a01610156565b909550935050604087013567ffffffffffffffff81111561021c575f5ffd5b61022889828a01610156565b979a9699509497509295939492505050565b5f5f5f5f5f5f6080878903121561024f575f5ffd5b863567ffffffffffffffff811115610265575f5ffd5b61027189828a01610156565b9097509550506020870135935060408701356001600160a01b0381168114610297575f5ffd5b9250606087013567ffffffffffffffff81111561021c575f5ffd5b5f600182016102cf57634e487b7160e01b5f52601160045260245ffd5b5060010190565b81835281816020850137505f828201602090810191909152601f909101601f19169091010190565b878152608060208201525f61031760808301888a6102d6565b828103604084015261032a8187896102d6565b9050828103606084015261033f8185876102d6565b9a9950505050505050505050565b818382375f9101908152919050565b608081525f61036f6080830187896102d6565b6020830195909552506001600160a01b0392909216604083015215156060909101529291505056fea26469706673582212206ec6712342a5df6257b9e87dbc0d614a2ba729966ea9af100eba022ff4c27f0f64736f6c634300081c0033",
}

// MockGMPABI is the input ABI used to generate the binding from.
// Deprecated: Use MockGMPMetaData.ABI instead.
var MockGMPABI = MockGMPMetaData.ABI

// MockGMPBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use MockGMPMetaData.Bin instead.
var MockGMPBin = MockGMPMetaData.Bin

// DeployMockGMP deploys a new Ethereum contract, binding an instance of MockGMP to it.
func DeployMockGMP(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *MockGMP, error) {
	parsed, err := MockGMPMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(MockGMPBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &MockGMP{MockGMPCaller: MockGMPCaller{contract: contract}, MockGMPTransactor: MockGMPTransactor{contract: contract}, MockGMPFilterer: MockGMPFilterer{contract: contract}}, nil
}

// MockGMP is an auto generated Go binding around an Ethereum contract.
type MockGMP struct {
	MockGMPCaller     // Read-only binding to the contract
	MockGMPTransactor // Write-only binding to the contract
	MockGMPFilterer   // Log filterer for contract events
}

// MockGMPCaller is an auto generated read-only Go binding around an Ethereum contract.
type MockGMPCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MockGMPTransactor is an auto generated write-only Go binding around an Ethereum contract.
type MockGMPTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MockGMPFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type MockGMPFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// MockGMPSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type MockGMPSession struct {
	Contract     *MockGMP          // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// MockGMPCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type MockGMPCallerSession struct {
	Contract *MockGMPCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts  // Call options to use throughout this session
}

// MockGMPTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type MockGMPTransactorSession struct {
	Contract     *MockGMPTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts  // Transaction auth options to use throughout this session
}

// MockGMPRaw is an auto generated low-level Go binding around an Ethereum contract.
type MockGMPRaw struct {
	Contract *MockGMP // Generic contract binding to access the raw methods on
}

// MockGMPCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type MockGMPCallerRaw struct {
	Contract *MockGMPCaller // Generic read-only contract binding to access the raw methods on
}

// MockGMPTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type MockGMPTransactorRaw struct {
	Contract *MockGMPTransactor // Generic write-only contract binding to access the raw methods on
}

// NewMockGMP creates a new instance of MockGMP, bound to a specific deployed contract.
func NewMockGMP(address common.Address, backend bind.ContractBackend) (*MockGMP, error) {
	contract, err := bindMockGMP(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &MockGMP{MockGMPCaller: MockGMPCaller{contract: contract}, MockGMPTransactor: MockGMPTransactor{contract: contract}, MockGMPFilterer: MockGMPFilterer{contract: contract}}, nil
}

// NewMockGMPCaller creates a new read-only instance of MockGMP, bound to a specific deployed contract.
func NewMockGMPCaller(address common.Address, caller bind.ContractCaller) (*MockGMPCaller, error) {
	contract, err := bindMockGMP(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &MockGMPCaller{contract: contract}, nil
}

// NewMockGMPTransactor creates a new write-only instance of MockGMP, bound to a specific deployed contract.
func NewMockGMPTransactor(address common.Address, transactor bind.ContractTransactor) (*MockGMPTransactor, error) {
	contract, err := bindMockGMP(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &MockGMPTransactor{contract: contract}, nil
}

// NewMockGMPFilterer creates a new log filterer instance of MockGMP, bound to a specific deployed contract.
func NewMockGMPFilterer(address common.Address, filterer bind.ContractFilterer) (*MockGMPFilterer, error) {
	contract, err := bindMockGMP(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &MockGMPFilterer{contract: contract}, nil
}

// bindMockGMP binds a generic wrapper to an already deployed contract.
func bindMockGMP(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := MockGMPMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_MockGMP *MockGMPRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _MockGMP.Contract.MockGMPCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_MockGMP *MockGMPRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _MockGMP.Contract.MockGMPTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_MockGMP *MockGMPRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _MockGMP.Contract.MockGMPTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_MockGMP *MockGMPCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _MockGMP.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_MockGMP *MockGMPTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _MockGMP.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_MockGMP *MockGMPTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _MockGMP.Contract.contract.Transact(opts, method, params...)
}

// Deliver is a paid mutator transaction binding the contract method 0xb6427b9c.
//
// Solidity: function deliver(string routeId, uint256 s, address target, bytes payload) returns()
func (_MockGMP *MockGMPTransactor) Deliver(opts *bind.TransactOpts, routeId string, s *big.Int, target common.Address, payload []byte) (*types.Transaction, error) {
	return _MockGMP.contract.Transact(opts, "deliver", routeId, s, target, payload)
}

// Deliver is a paid mutator transaction binding the contract method 0xb6427b9c.
//
// Solidity: function deliver(string routeId, uint256 s, address target, bytes payload) returns()
func (_MockGMP *MockGMPSession) Deliver(routeId string, s *big.Int, target common.Address, payload []byte) (*types.Transaction, error) {
	return _MockGMP.Contract.Deliver(&_MockGMP.TransactOpts, routeId, s, target, payload)
}

// Deliver is a paid mutator transaction binding the contract method 0xb6427b9c.
//
// Solidity: function deliver(string routeId, uint256 s, address target, bytes payload) returns()
func (_MockGMP *MockGMPTransactorSession) Deliver(routeId string, s *big.Int, target common.Address, payload []byte) (*types.Transaction, error) {
	return _MockGMP.Contract.Deliver(&_MockGMP.TransactOpts, routeId, s, target, payload)
}

// Send is a paid mutator transaction binding the contract method 0x1e0d43b9.
//
// Solidity: function send(string routeId, string target, bytes payload) returns()
func (_MockGMP *MockGMPTransactor) Send(opts *bind.TransactOpts, routeId string, target string, payload []byte) (*types.Transaction, error) {
	return _MockGMP.contract.Transact(opts, "send", routeId, target, payload)
}

// Send is a paid mutator transaction binding the contract method 0x1e0d43b9.
//
// Solidity: function send(string routeId, string target, bytes payload) returns()
func (_MockGMP *MockGMPSession) Send(routeId string, target string, payload []byte) (*types.Transaction, error) {
	return _MockGMP.Contract.Send(&_MockGMP.TransactOpts, routeId, target, payload)
}

// Send is a paid mutator transaction binding the contract method 0x1e0d43b9.
//
// Solidity: function send(string routeId, string target, bytes payload) returns()
func (_MockGMP *MockGMPTransactorSession) Send(routeId string, target string, payload []byte) (*types.Transaction, error) {
	return _MockGMP.Contract.Send(&_MockGMP.TransactOpts, routeId, target, payload)
}

// MockGMPGMPReceivedIterator is returned from FilterGMPReceived and is used to iterate over the raw logs and unpacked data for GMPReceived events raised by the MockGMP contract.
type MockGMPGMPReceivedIterator struct {
	Event *MockGMPGMPReceived // Event containing the contract specifics and raw log

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
func (it *MockGMPGMPReceivedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MockGMPGMPReceived)
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
		it.Event = new(MockGMPGMPReceived)
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
func (it *MockGMPGMPReceivedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MockGMPGMPReceivedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MockGMPGMPReceived represents a GMPReceived event raised by the MockGMP contract.
type MockGMPGMPReceived struct {
	RouteId string
	Seq     *big.Int
	Target  common.Address
	Success bool
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterGMPReceived is a free log retrieval operation binding the contract event 0xd7fdb4bb9c364fe7b30d3fe638a8b3f3bae426dee14cab821f4297b17cb6fbd9.
//
// Solidity: event GMPReceived(string routeId, uint256 seq, address target, bool success)
func (_MockGMP *MockGMPFilterer) FilterGMPReceived(opts *bind.FilterOpts) (*MockGMPGMPReceivedIterator, error) {

	logs, sub, err := _MockGMP.contract.FilterLogs(opts, "GMPReceived")
	if err != nil {
		return nil, err
	}
	return &MockGMPGMPReceivedIterator{contract: _MockGMP.contract, event: "GMPReceived", logs: logs, sub: sub}, nil
}

// WatchGMPReceived is a free log subscription operation binding the contract event 0xd7fdb4bb9c364fe7b30d3fe638a8b3f3bae426dee14cab821f4297b17cb6fbd9.
//
// Solidity: event GMPReceived(string routeId, uint256 seq, address target, bool success)
func (_MockGMP *MockGMPFilterer) WatchGMPReceived(opts *bind.WatchOpts, sink chan<- *MockGMPGMPReceived) (event.Subscription, error) {

	logs, sub, err := _MockGMP.contract.WatchLogs(opts, "GMPReceived")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MockGMPGMPReceived)
				if err := _MockGMP.contract.UnpackLog(event, "GMPReceived", log); err != nil {
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

// ParseGMPReceived is a log parse operation binding the contract event 0xd7fdb4bb9c364fe7b30d3fe638a8b3f3bae426dee14cab821f4297b17cb6fbd9.
//
// Solidity: event GMPReceived(string routeId, uint256 seq, address target, bool success)
func (_MockGMP *MockGMPFilterer) ParseGMPReceived(log types.Log) (*MockGMPGMPReceived, error) {
	event := new(MockGMPGMPReceived)
	if err := _MockGMP.contract.UnpackLog(event, "GMPReceived", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// MockGMPGMPSentIterator is returned from FilterGMPSent and is used to iterate over the raw logs and unpacked data for GMPSent events raised by the MockGMP contract.
type MockGMPGMPSentIterator struct {
	Event *MockGMPGMPSent // Event containing the contract specifics and raw log

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
func (it *MockGMPGMPSentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(MockGMPGMPSent)
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
		it.Event = new(MockGMPGMPSent)
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
func (it *MockGMPGMPSentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *MockGMPGMPSentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// MockGMPGMPSent represents a GMPSent event raised by the MockGMP contract.
type MockGMPGMPSent struct {
	Seq     *big.Int
	RouteId string
	Target  string
	Payload []byte
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterGMPSent is a free log retrieval operation binding the contract event 0x0ffa85d772b952085fe1134bf99ab2e93f970b4f6ee17d4304a04a03cdffb04f.
//
// Solidity: event GMPSent(uint256 seq, string routeId, string target, bytes payload)
func (_MockGMP *MockGMPFilterer) FilterGMPSent(opts *bind.FilterOpts) (*MockGMPGMPSentIterator, error) {

	logs, sub, err := _MockGMP.contract.FilterLogs(opts, "GMPSent")
	if err != nil {
		return nil, err
	}
	return &MockGMPGMPSentIterator{contract: _MockGMP.contract, event: "GMPSent", logs: logs, sub: sub}, nil
}

// WatchGMPSent is a free log subscription operation binding the contract event 0x0ffa85d772b952085fe1134bf99ab2e93f970b4f6ee17d4304a04a03cdffb04f.
//
// Solidity: event GMPSent(uint256 seq, string routeId, string target, bytes payload)
func (_MockGMP *MockGMPFilterer) WatchGMPSent(opts *bind.WatchOpts, sink chan<- *MockGMPGMPSent) (event.Subscription, error) {

	logs, sub, err := _MockGMP.contract.WatchLogs(opts, "GMPSent")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(MockGMPGMPSent)
				if err := _MockGMP.contract.UnpackLog(event, "GMPSent", log); err != nil {
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

// ParseGMPSent is a log parse operation binding the contract event 0x0ffa85d772b952085fe1134bf99ab2e93f970b4f6ee17d4304a04a03cdffb04f.
//
// Solidity: event GMPSent(uint256 seq, string routeId, string target, bytes payload)
func (_MockGMP *MockGMPFilterer) ParseGMPSent(log types.Log) (*MockGMPGMPSent, error) {
	event := new(MockGMPGMPSent)
	if err := _MockGMP.contract.UnpackLog(event, "GMPSent", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
