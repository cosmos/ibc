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

// TestAppDeployerMetaData contains all meta data concerning the TestAppDeployer contract.
var TestAppDeployerMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"initialIFTSupply\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"TestAppsDeployed\",\"inputs\":[{\"name\":\"mockGMP\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"mockIFT\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"counter\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false}]",
	Bin: "0x608060405234801561000f575f5ffd5b50604051610dc4380380610dc483398101604081905261002e91610180565b5f60405161003b9061015a565b604051809103905ff080158015610054573d5f5f3e3d5ffd5b5090505f60405161006490610167565b604051809103905ff08015801561007d573d5f5f3e3d5ffd5b5090505f60405161008d90610174565b604051809103905ff0801580156100a6573d5f5f3e3d5ffd5b506040516340c10f1960e01b8152336004820152602481018690529091506001600160a01b038316906340c10f19906044015f604051808303815f87803b1580156100ef575f5ffd5b505af1158015610101573d5f5f3e3d5ffd5b5050604080516001600160a01b038781168252868116602083015285168183015290517f6598c11a9669f91f86a8a07f15da037df79f7d34f4691da856318a0006c7c8289350908190036060019150a150505050610197565b6103e9806101e183390190565b610720806105ca83390190565b60da80610cea83390190565b5f60208284031215610190575f5ffd5b5051919050565b603e806101a35f395ff3fe60806040525f5ffdfea26469706673582212204775cf69b7fcb77ea92c6d4e01299ef089a86a689e3e551d8d2a7b5d23a6e3cc64736f6c634300081c00336080604052348015600e575f5ffd5b506103cd8061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610034575f3560e01c80631e0d43b914610038578063b6427b9c1461004d575b5f5ffd5b61004b61004636600461019b565b610060565b005b61004b61005b36600461023a565b6100c2565b5f5f5f815461006e906102b2565b91905081905590507f0ffa85d772b952085fe1134bf99ab2e93f970b4f6ee17d4304a04a03cdffb04f818888888888886040516100b197969594939291906102fe565b60405180910390a150505050505050565b5f836001600160a01b031683836040516100dd92919061034d565b5f604051808303815f865af19150503d805f8114610116576040519150601f19603f3d011682016040523d82523d5f602084013e61011b565b606091505b505090507fd7fdb4bb9c364fe7b30d3fe638a8b3f3bae426dee14cab821f4297b17cb6fbd987878787856040516100b195949392919061035c565b5f5f83601f840112610166575f5ffd5b50813567ffffffffffffffff81111561017d575f5ffd5b602083019150836020828501011115610194575f5ffd5b9250929050565b5f5f5f5f5f5f606087890312156101b0575f5ffd5b863567ffffffffffffffff8111156101c6575f5ffd5b6101d289828a01610156565b909750955050602087013567ffffffffffffffff8111156101f1575f5ffd5b6101fd89828a01610156565b909550935050604087013567ffffffffffffffff81111561021c575f5ffd5b61022889828a01610156565b979a9699509497509295939492505050565b5f5f5f5f5f5f6080878903121561024f575f5ffd5b863567ffffffffffffffff811115610265575f5ffd5b61027189828a01610156565b9097509550506020870135935060408701356001600160a01b0381168114610297575f5ffd5b9250606087013567ffffffffffffffff81111561021c575f5ffd5b5f600182016102cf57634e487b7160e01b5f52601160045260245ffd5b5060010190565b81835281816020850137505f828201602090810191909152601f909101601f19169091010190565b878152608060208201525f61031760808301888a6102d6565b828103604084015261032a8187896102d6565b9050828103606084015261033f8185876102d6565b9a9950505050505050505050565b818382375f9101908152919050565b608081525f61036f6080830187896102d6565b6020830195909552506001600160a01b0392909216604083015215156060909101529291505056fea26469706673582212206ec6712342a5df6257b9e87dbc0d614a2ba729966ea9af100eba022ff4c27f0f64736f6c634300081c00336080604052348015600e575f5ffd5b506107048061001c5f395ff3fe608060405234801561000f575f5ffd5b5060043610610055575f3560e01c8063278ecde11461005957806340c10f191461006e57806358d46e891461008157806370a082311461009457806389185769146100c5575b5f5ffd5b61006c610067366004610436565b6100d8565b005b61006c61007c366004610468565b610247565b61006c61008f3660046104d5565b610277565b6100b36100a2366004610535565b5f6020819052908152604090205481565b60405190815260200160405180910390f35b61006c6100d3366004610555565b6102ea565b5f81815260026020526040902060018101546101275760405162461bcd60e51b81526020600482015260096024820152686e6f20657363726f7760b81b60448201526064015b60405180910390fd5b80600201545f0361016b5760405162461bcd60e51b815260206004820152600e60248201526d1b9bc81d1a5b595bdd5d081cd95d60921b604482015260640161011e565b600381015460ff16156101b35760405162461bcd60e51b815260206004820152601060248201526f185b1c9958591e481c99599d5b99195960821b604482015260640161011e565b60038101805460ff1916600190811790915581015481546001600160a01b03165f90815260208190526040812080549091906101f09084906105e6565b909155505080546001820154604080518581526001600160a01b0390931660208401528201527f56987a7c1f738a2628ad3a97d8dd0d3d39cf208b5b95f2f8488c924ee7a857f09060600160405180910390a15050565b6001600160a01b0382165f908152602081905260408120805483929061026e9084906105e6565b90915550505050565b6001600160a01b0382165f908152602081905260408120805483929061029e9084906105e6565b90915550506040517fb5df872fe8db758a69d0f8500566a823f40dc55b858395ae89a58689ed51cfdb906102db9087908790879087908790610627565b60405180910390a15050505050565b335f9081526020819052604090205482111561033f5760405162461bcd60e51b8152602060048201526014602482015273696e73756666696369656e742062616c616e636560601b604482015260640161011e565b335f908152602081905260408120805484929061035d908490610660565b925050819055505f60015f815461037390610673565b91829055506040805160808101825233815260208082018781528284018781525f6060850181815287825260029485905290869020945185546001600160a01b0319166001600160a01b03909116178555915160018501555191830191909155516003909101805460ff1916911515919091179055519091507fa46e3879c3d5957518251cfe90f7589d3e38c7ebb4f4ac2b46bf8d58fcbb3709906104259083908a908a908a908a908a908a9061068b565b60405180910390a150505050505050565b5f60208284031215610446575f5ffd5b5035919050565b80356001600160a01b0381168114610463575f5ffd5b919050565b5f5f60408385031215610479575f5ffd5b6104828361044d565b946020939093013593505050565b5f5f83601f8401126104a0575f5ffd5b50813567ffffffffffffffff8111156104b7575f5ffd5b6020830191508360208285010111156104ce575f5ffd5b9250929050565b5f5f5f5f5f608086880312156104e9575f5ffd5b853567ffffffffffffffff8111156104ff575f5ffd5b61050b88828901610490565b909650945050602086013592506105246040870161044d565b949793965091946060013592915050565b5f60208284031215610545575f5ffd5b61054e8261044d565b9392505050565b5f5f5f5f5f5f6080878903121561056a575f5ffd5b863567ffffffffffffffff811115610580575f5ffd5b61058c89828a01610490565b909750955050602087013567ffffffffffffffff8111156105ab575f5ffd5b6105b789828a01610490565b979a9699509760408101359660609091013595509350505050565b634e487b7160e01b5f52601160045260245ffd5b808201808211156105f9576105f96105d2565b92915050565b81835281816020850137505f828201602090810191909152601f909101601f19169091010190565b608081525f61063a6080830187896105ff565b6020830195909552506001600160a01b0392909216604083015260609091015292915050565b818103818111156105f9576105f96105d2565b5f60018201610684576106846105d2565b5060010190565b87815260a060208201525f6106a460a08301888a6105ff565b82810360408401526106b78187896105ff565b60608401959095525050608001529594505050505056fea2646970667358221220cb391dc343b421d918e2e47016eb69321dbcfdf0dc885e069797f5f3d350728964736f6c634300081c00336080604052348015600e575f5ffd5b5060c080601a5f395ff3fe6080604052348015600e575f5ffd5b50600436106030575f3560e01c806306661abd146034578063d09de08a146048575b5f5ffd5b5f5460405190815260200160405180910390f35b604e6050565b005b60015f5f828254605f91906066565b9091555050565b80820180821115608457634e487b7160e01b5f52601160045260245ffd5b9291505056fea2646970667358221220f1d8426841b3c2174575e381c0a07f8a552a845601301917a88a09b1dcef895f64736f6c634300081c0033",
}

// TestAppDeployerABI is the input ABI used to generate the binding from.
// Deprecated: Use TestAppDeployerMetaData.ABI instead.
var TestAppDeployerABI = TestAppDeployerMetaData.ABI

// TestAppDeployerBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use TestAppDeployerMetaData.Bin instead.
var TestAppDeployerBin = TestAppDeployerMetaData.Bin

// DeployTestAppDeployer deploys a new Ethereum contract, binding an instance of TestAppDeployer to it.
func DeployTestAppDeployer(auth *bind.TransactOpts, backend bind.ContractBackend, initialIFTSupply *big.Int) (common.Address, *types.Transaction, *TestAppDeployer, error) {
	parsed, err := TestAppDeployerMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(TestAppDeployerBin), backend, initialIFTSupply)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &TestAppDeployer{TestAppDeployerCaller: TestAppDeployerCaller{contract: contract}, TestAppDeployerTransactor: TestAppDeployerTransactor{contract: contract}, TestAppDeployerFilterer: TestAppDeployerFilterer{contract: contract}}, nil
}

// TestAppDeployer is an auto generated Go binding around an Ethereum contract.
type TestAppDeployer struct {
	TestAppDeployerCaller     // Read-only binding to the contract
	TestAppDeployerTransactor // Write-only binding to the contract
	TestAppDeployerFilterer   // Log filterer for contract events
}

// TestAppDeployerCaller is an auto generated read-only Go binding around an Ethereum contract.
type TestAppDeployerCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TestAppDeployerTransactor is an auto generated write-only Go binding around an Ethereum contract.
type TestAppDeployerTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TestAppDeployerFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type TestAppDeployerFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// TestAppDeployerSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type TestAppDeployerSession struct {
	Contract     *TestAppDeployer  // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// TestAppDeployerCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type TestAppDeployerCallerSession struct {
	Contract *TestAppDeployerCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts          // Call options to use throughout this session
}

// TestAppDeployerTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type TestAppDeployerTransactorSession struct {
	Contract     *TestAppDeployerTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts          // Transaction auth options to use throughout this session
}

// TestAppDeployerRaw is an auto generated low-level Go binding around an Ethereum contract.
type TestAppDeployerRaw struct {
	Contract *TestAppDeployer // Generic contract binding to access the raw methods on
}

// TestAppDeployerCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type TestAppDeployerCallerRaw struct {
	Contract *TestAppDeployerCaller // Generic read-only contract binding to access the raw methods on
}

// TestAppDeployerTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type TestAppDeployerTransactorRaw struct {
	Contract *TestAppDeployerTransactor // Generic write-only contract binding to access the raw methods on
}

// NewTestAppDeployer creates a new instance of TestAppDeployer, bound to a specific deployed contract.
func NewTestAppDeployer(address common.Address, backend bind.ContractBackend) (*TestAppDeployer, error) {
	contract, err := bindTestAppDeployer(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &TestAppDeployer{TestAppDeployerCaller: TestAppDeployerCaller{contract: contract}, TestAppDeployerTransactor: TestAppDeployerTransactor{contract: contract}, TestAppDeployerFilterer: TestAppDeployerFilterer{contract: contract}}, nil
}

// NewTestAppDeployerCaller creates a new read-only instance of TestAppDeployer, bound to a specific deployed contract.
func NewTestAppDeployerCaller(address common.Address, caller bind.ContractCaller) (*TestAppDeployerCaller, error) {
	contract, err := bindTestAppDeployer(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &TestAppDeployerCaller{contract: contract}, nil
}

// NewTestAppDeployerTransactor creates a new write-only instance of TestAppDeployer, bound to a specific deployed contract.
func NewTestAppDeployerTransactor(address common.Address, transactor bind.ContractTransactor) (*TestAppDeployerTransactor, error) {
	contract, err := bindTestAppDeployer(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &TestAppDeployerTransactor{contract: contract}, nil
}

// NewTestAppDeployerFilterer creates a new log filterer instance of TestAppDeployer, bound to a specific deployed contract.
func NewTestAppDeployerFilterer(address common.Address, filterer bind.ContractFilterer) (*TestAppDeployerFilterer, error) {
	contract, err := bindTestAppDeployer(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &TestAppDeployerFilterer{contract: contract}, nil
}

// bindTestAppDeployer binds a generic wrapper to an already deployed contract.
func bindTestAppDeployer(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := TestAppDeployerMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_TestAppDeployer *TestAppDeployerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _TestAppDeployer.Contract.TestAppDeployerCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_TestAppDeployer *TestAppDeployerRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TestAppDeployer.Contract.TestAppDeployerTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_TestAppDeployer *TestAppDeployerRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _TestAppDeployer.Contract.TestAppDeployerTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_TestAppDeployer *TestAppDeployerCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _TestAppDeployer.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_TestAppDeployer *TestAppDeployerTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _TestAppDeployer.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_TestAppDeployer *TestAppDeployerTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _TestAppDeployer.Contract.contract.Transact(opts, method, params...)
}

// TestAppDeployerTestAppsDeployedIterator is returned from FilterTestAppsDeployed and is used to iterate over the raw logs and unpacked data for TestAppsDeployed events raised by the TestAppDeployer contract.
type TestAppDeployerTestAppsDeployedIterator struct {
	Event *TestAppDeployerTestAppsDeployed // Event containing the contract specifics and raw log

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
func (it *TestAppDeployerTestAppsDeployedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(TestAppDeployerTestAppsDeployed)
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
		it.Event = new(TestAppDeployerTestAppsDeployed)
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
func (it *TestAppDeployerTestAppsDeployedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *TestAppDeployerTestAppsDeployedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// TestAppDeployerTestAppsDeployed represents a TestAppsDeployed event raised by the TestAppDeployer contract.
type TestAppDeployerTestAppsDeployed struct {
	MockGMP common.Address
	MockIFT common.Address
	Counter common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterTestAppsDeployed is a free log retrieval operation binding the contract event 0x6598c11a9669f91f86a8a07f15da037df79f7d34f4691da856318a0006c7c828.
//
// Solidity: event TestAppsDeployed(address mockGMP, address mockIFT, address counter)
func (_TestAppDeployer *TestAppDeployerFilterer) FilterTestAppsDeployed(opts *bind.FilterOpts) (*TestAppDeployerTestAppsDeployedIterator, error) {

	logs, sub, err := _TestAppDeployer.contract.FilterLogs(opts, "TestAppsDeployed")
	if err != nil {
		return nil, err
	}
	return &TestAppDeployerTestAppsDeployedIterator{contract: _TestAppDeployer.contract, event: "TestAppsDeployed", logs: logs, sub: sub}, nil
}

// WatchTestAppsDeployed is a free log subscription operation binding the contract event 0x6598c11a9669f91f86a8a07f15da037df79f7d34f4691da856318a0006c7c828.
//
// Solidity: event TestAppsDeployed(address mockGMP, address mockIFT, address counter)
func (_TestAppDeployer *TestAppDeployerFilterer) WatchTestAppsDeployed(opts *bind.WatchOpts, sink chan<- *TestAppDeployerTestAppsDeployed) (event.Subscription, error) {

	logs, sub, err := _TestAppDeployer.contract.WatchLogs(opts, "TestAppsDeployed")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(TestAppDeployerTestAppsDeployed)
				if err := _TestAppDeployer.contract.UnpackLog(event, "TestAppsDeployed", log); err != nil {
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

// ParseTestAppsDeployed is a log parse operation binding the contract event 0x6598c11a9669f91f86a8a07f15da037df79f7d34f4691da856318a0006c7c828.
//
// Solidity: event TestAppsDeployed(address mockGMP, address mockIFT, address counter)
func (_TestAppDeployer *TestAppDeployerFilterer) ParseTestAppsDeployed(log types.Log) (*TestAppDeployerTestAppsDeployed, error) {
	event := new(TestAppDeployerTestAppsDeployed)
	if err := _TestAppDeployer.contract.UnpackLog(event, "TestAppsDeployed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
