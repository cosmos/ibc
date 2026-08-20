// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package escrow

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

// EscrowMetaData contains all meta data concerning the Escrow contract.
var EscrowMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"authority\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDailyUsage\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRateLimit\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"ics20\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[{\"name\":\"ics20_\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"authority\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"initializeV2\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isConsumingScheduledOp\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"recvCallback\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"send\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"contractIERC20\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setAuthority\",\"inputs\":[{\"name\":\"newAuthority\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setRateLimit\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"rateLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AuthorityUpdated\",\"inputs\":[{\"name\":\"authority\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AccessManagedInvalidAuthority\",\"inputs\":[{\"name\":\"authority\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"AccessManagedRequiredDelay\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"delay\",\"type\":\"uint32\",\"internalType\":\"uint32\"}]},{\"type\":\"error\",\"name\":\"AccessManagedUnauthorized\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"EscrowUnauthorized\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"RateLimitExceeded\",\"inputs\":[{\"name\":\"rateLimit\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"usage\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
	Bin: "0x6080806040523460aa575f5160206112595f395f51905f525460ff8160401c16609b576002600160401b03196001600160401b038216016049575b6040516111aa90816100af8239f35b6001600160401b0319166001600160401b039081175f5160206112595f395f51905f525581527fc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d290602090a15f80603a565b63f92ee8a960e01b5f5260045ffd5b5f80fdfe60806040526004361015610011575f80fd5b5f5f358060e01c9081630779afe614610c025781630b0aee6914610b9e578163485cc955146109275781635cd8a76b146106415781637a9e5e4b146105985781638fb36037146105055781639a61e8e1146104b3578163b4f22eb714610440578163bf7e214f146103ee578163d34a3fd9146100ee575063f11d5ea914610096575f80fd5b346100eb5760206003193601126100eb5760406020916100bc6100b7610d60565b6110f7565b81527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10183522054604051908152f35b80fd5b9050346103ab5760406003193601126103ab57610109610d60565b907ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a005473ffffffffffffffffffffffffffffffffffffffff811691366004116103ab575f60405f809382517fffffffff0000000000000000000000000000000000000000000000000000000060208201927fb7009613000000000000000000000000000000000000000000000000000000008452336024840152306044840152166064820152606481526101be608482610dcd565b828052826020525190875afa6103db575b1561021e575b8473ffffffffffffffffffffffffffffffffffffffff851681527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c100602052602435604082205580f35b63ffffffff16156103af577fffffffffffffffffffffff00ffffffffffffffffffffffffffffffffffffffff1674010000000000000000000000000000000000000000177ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a0055803b156103ab575f60405180927f94c7d7ee0000000000000000000000000000000000000000000000000000000082523360048301526040602483015236604483015236836064840137826064368401015281836064827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0601f36011681010301925af180156103a057610377575b507ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a0080547fffffffffffffffffffffff00ffffffffffffffffffffffffffffffffffffffff16905573ffffffffffffffffffffffffffffffffffffffff5f806101d5565b6103849192505f90610dcd565b5f9073ffffffffffffffffffffffffffffffffffffffff610313565b6040513d5f823e3d90fd5b5f80fd5b7f068ca9d8000000000000000000000000000000000000000000000000000000005f523360045260245ffd5b50505f516020518060201c1502906101cf565b346103ab575f6003193601126103ab57602073ffffffffffffffffffffffffffffffffffffffff7ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a005416604051908152f35b346103ab5760606003193601126103ab576104b161045c610d60565b610464610d3d565b506104a83373ffffffffffffffffffffffffffffffffffffffff7f537eb9d931756581e7ea6f7811162c646321946650ac0ac6bf83b24932e4160054163314610d83565b60443590611006565b005b346103ab575f6003193601126103ab57602073ffffffffffffffffffffffffffffffffffffffff7f537eb9d931756581e7ea6f7811162c646321946650ac0ac6bf83b24932e416005416604051908152f35b346103ab575f6003193601126103ab577ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a005460a01c60ff16156105905760207f8fb36037000000000000000000000000000000000000000000000000000000005b7fffffffff0000000000000000000000000000000000000000000000000000000060405191168152f35b60205f610566565b346103ab5760206003193601126103ab576105b1610d60565b73ffffffffffffffffffffffffffffffffffffffff7ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a00541633036103af57803b156105ff576104b190610f5b565b73ffffffffffffffffffffffffffffffffffffffff907fc2f31e5e000000000000000000000000000000000000000000000000000000005f521660045260245ffd5b346103ab575f6003193601126103ab577ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005467ffffffffffffffff811690600182036108f35760401c60ff1690811561091b575b506108f35761070860027fffffffffffffffffffffffffffffffffffffffffffffffff00000000000000007ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005416177ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a0055565b680100000000000000007fffffffffffffffffffffffffffffffffffffffffffffff00ffffffffffffffff7ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005416177ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a00556004602073ffffffffffffffffffffffffffffffffffffffff7f537eb9d931756581e7ea6f7811162c646321946650ac0ac6bf83b24932e416005416604051928380927fbf7e214f0000000000000000000000000000000000000000000000000000000082525afa80156103a0575f906108a2575b61080f906107fa611146565b610802611146565b61080a611146565b610f5b565b7fffffffffffffffffffffffffffffffffffffffffffffff00ffffffffffffffff7ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a0054167ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a00557fc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2602060405160028152a1005b506020813d6020116108eb575b816108bc60209383610dcd565b810103126103ab575173ffffffffffffffffffffffffffffffffffffffff811681036103ab5761080f906107ee565b3d91506108af565b7ff92ee8a9000000000000000000000000000000000000000000000000000000005f5260045ffd5b60029150101581610695565b346103ab5760406003193601126103ab57610940610d60565b610948610d3d565b907ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005467ffffffffffffffff811690816108f35760401c60ff16908115610b92575b506108f357610a9773ffffffffffffffffffffffffffffffffffffffff92610a1660027fffffffffffffffffffffffffffffffffffffffffffffffff00000000000000007ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005416177ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a0055565b680100000000000000007fffffffffffffffffffffffffffffffffffffffffffffff00ffffffffffffffff7ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005416177ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a0055610a8f611146565b6107fa611146565b167fffffffffffffffffffffffff00000000000000000000000000000000000000007f537eb9d931756581e7ea6f7811162c646321946650ac0ac6bf83b24932e416005416177f537eb9d931756581e7ea6f7811162c646321946650ac0ac6bf83b24932e41600557fffffffffffffffffffffffffffffffffffffffffffffff00ffffffffffffffff7ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a0054167ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a00557fc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2602060405160028152a1005b6002915010158361098a565b346103ab5760206003193601126103ab5773ffffffffffffffffffffffffffffffffffffffff610bcc610d60565b165f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c100602052602060405f2054604051908152f35b346103ab5760606003193601126103ab5760043573ffffffffffffffffffffffffffffffffffffffff8116908181036103ab57610c3d610d3d565b90604435610c843373ffffffffffffffffffffffffffffffffffffffff7f537eb9d931756581e7ea6f7811162c646321946650ac0ac6bf83b24932e4160054163314610d83565b610c8e8185610e3b565b73ffffffffffffffffffffffffffffffffffffffff604051937fa9059cbb000000000000000000000000000000000000000000000000000000005f521660045260245260205f60448180855af19160015f5114831615610d1d575b505015610cf257005b7f5274afe7000000000000000000000000000000000000000000000000000000005f5260045260245ffd5b6001831516610d3557503b15153d1516168280610ce9565b3d5f823e3d90fd5b6024359073ffffffffffffffffffffffffffffffffffffffff821682036103ab57565b6004359073ffffffffffffffffffffffffffffffffffffffff821682036103ab57565b15610d8b5750565b73ffffffffffffffffffffffffffffffffffffffff907facd411fe000000000000000000000000000000000000000000000000000000005f521660045260245ffd5b90601f7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0910116810190811067ffffffffffffffff821117610e0e57604052565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52604160045260245ffd5b73ffffffffffffffffffffffffffffffffffffffff81165f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10060205260405f2054908115610f5657610e8d906110f7565b90815f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10160205260405f2054928301809311610f2957808311610ef957505f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10160205260405f2055565b90507f5f713586000000000000000000000000000000000000000000000000000000005f5260045260245260445ffd5b7f4e487b71000000000000000000000000000000000000000000000000000000005f52601160045260245ffd5b505050565b602073ffffffffffffffffffffffffffffffffffffffff7f2f658b440c35314f52658ea8a740e05b284cdc84dc9ae01e891f21b8933e7cad9216807fffffffffffffffffffffffff00000000000000000000000000000000000000007ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a005416177ff3177357ab46d8af007ab3fdb9af81da189e1068fefdc0073dca88a2cab40a0055604051908152a1565b73ffffffffffffffffffffffffffffffffffffffff81165f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10060205260405f2054156110f357611056906110f7565b805f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10160205260405f2054918083115f146110c2578203918211610f29575f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c10160205260405f2055565b5090505f527fcb05b6cb8e6c87c443cb04d44193d7d46d51c1198725a0ee3478d5baa736c1016020525f6040812055565b5050565b6040517fffffffffffffffffffffffffffffffffffffffff0000000000000000000000006020820192620151804204845260601b16604082015260348152611140605482610dcd565b51902090565b60ff7ff0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a005460401c161561117557565b7fd7e6bcf8000000000000000000000000000000000000000000000000000000005f5260045ffdfea164736f6c634300081c000af0c57e16840df040f15088dc2f81fe391c3923bec73e23a9662efc9c229c6a00",
}

// EscrowABI is the input ABI used to generate the binding from.
// Deprecated: Use EscrowMetaData.ABI instead.
var EscrowABI = EscrowMetaData.ABI

// EscrowBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use EscrowMetaData.Bin instead.
var EscrowBin = EscrowMetaData.Bin

// DeployEscrow deploys a new Ethereum contract, binding an instance of Escrow to it.
func DeployEscrow(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *Escrow, error) {
	parsed, err := EscrowMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(EscrowBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &Escrow{EscrowCaller: EscrowCaller{contract: contract}, EscrowTransactor: EscrowTransactor{contract: contract}, EscrowFilterer: EscrowFilterer{contract: contract}}, nil
}

// Escrow is an auto generated Go binding around an Ethereum contract.
type Escrow struct {
	EscrowCaller     // Read-only binding to the contract
	EscrowTransactor // Write-only binding to the contract
	EscrowFilterer   // Log filterer for contract events
}

// EscrowCaller is an auto generated read-only Go binding around an Ethereum contract.
type EscrowCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// EscrowTransactor is an auto generated write-only Go binding around an Ethereum contract.
type EscrowTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// EscrowFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type EscrowFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// EscrowSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type EscrowSession struct {
	Contract     *Escrow           // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// EscrowCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type EscrowCallerSession struct {
	Contract *EscrowCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// EscrowTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type EscrowTransactorSession struct {
	Contract     *EscrowTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// EscrowRaw is an auto generated low-level Go binding around an Ethereum contract.
type EscrowRaw struct {
	Contract *Escrow // Generic contract binding to access the raw methods on
}

// EscrowCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type EscrowCallerRaw struct {
	Contract *EscrowCaller // Generic read-only contract binding to access the raw methods on
}

// EscrowTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type EscrowTransactorRaw struct {
	Contract *EscrowTransactor // Generic write-only contract binding to access the raw methods on
}

// NewEscrow creates a new instance of Escrow, bound to a specific deployed contract.
func NewEscrow(address common.Address, backend bind.ContractBackend) (*Escrow, error) {
	contract, err := bindEscrow(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Escrow{EscrowCaller: EscrowCaller{contract: contract}, EscrowTransactor: EscrowTransactor{contract: contract}, EscrowFilterer: EscrowFilterer{contract: contract}}, nil
}

// NewEscrowCaller creates a new read-only instance of Escrow, bound to a specific deployed contract.
func NewEscrowCaller(address common.Address, caller bind.ContractCaller) (*EscrowCaller, error) {
	contract, err := bindEscrow(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &EscrowCaller{contract: contract}, nil
}

// NewEscrowTransactor creates a new write-only instance of Escrow, bound to a specific deployed contract.
func NewEscrowTransactor(address common.Address, transactor bind.ContractTransactor) (*EscrowTransactor, error) {
	contract, err := bindEscrow(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &EscrowTransactor{contract: contract}, nil
}

// NewEscrowFilterer creates a new log filterer instance of Escrow, bound to a specific deployed contract.
func NewEscrowFilterer(address common.Address, filterer bind.ContractFilterer) (*EscrowFilterer, error) {
	contract, err := bindEscrow(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &EscrowFilterer{contract: contract}, nil
}

// bindEscrow binds a generic wrapper to an already deployed contract.
func bindEscrow(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := EscrowMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Escrow *EscrowRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Escrow.Contract.EscrowCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Escrow *EscrowRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Escrow.Contract.EscrowTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Escrow *EscrowRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Escrow.Contract.EscrowTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Escrow *EscrowCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Escrow.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Escrow *EscrowTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Escrow.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Escrow *EscrowTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Escrow.Contract.contract.Transact(opts, method, params...)
}

// Authority is a free data retrieval call binding the contract method 0xbf7e214f.
//
// Solidity: function authority() view returns(address)
func (_Escrow *EscrowCaller) Authority(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Escrow.contract.Call(opts, &out, "authority")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Authority is a free data retrieval call binding the contract method 0xbf7e214f.
//
// Solidity: function authority() view returns(address)
func (_Escrow *EscrowSession) Authority() (common.Address, error) {
	return _Escrow.Contract.Authority(&_Escrow.CallOpts)
}

// Authority is a free data retrieval call binding the contract method 0xbf7e214f.
//
// Solidity: function authority() view returns(address)
func (_Escrow *EscrowCallerSession) Authority() (common.Address, error) {
	return _Escrow.Contract.Authority(&_Escrow.CallOpts)
}

// GetDailyUsage is a free data retrieval call binding the contract method 0xf11d5ea9.
//
// Solidity: function getDailyUsage(address token) view returns(uint256)
func (_Escrow *EscrowCaller) GetDailyUsage(opts *bind.CallOpts, token common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Escrow.contract.Call(opts, &out, "getDailyUsage", token)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDailyUsage is a free data retrieval call binding the contract method 0xf11d5ea9.
//
// Solidity: function getDailyUsage(address token) view returns(uint256)
func (_Escrow *EscrowSession) GetDailyUsage(token common.Address) (*big.Int, error) {
	return _Escrow.Contract.GetDailyUsage(&_Escrow.CallOpts, token)
}

// GetDailyUsage is a free data retrieval call binding the contract method 0xf11d5ea9.
//
// Solidity: function getDailyUsage(address token) view returns(uint256)
func (_Escrow *EscrowCallerSession) GetDailyUsage(token common.Address) (*big.Int, error) {
	return _Escrow.Contract.GetDailyUsage(&_Escrow.CallOpts, token)
}

// GetRateLimit is a free data retrieval call binding the contract method 0x0b0aee69.
//
// Solidity: function getRateLimit(address token) view returns(uint256)
func (_Escrow *EscrowCaller) GetRateLimit(opts *bind.CallOpts, token common.Address) (*big.Int, error) {
	var out []interface{}
	err := _Escrow.contract.Call(opts, &out, "getRateLimit", token)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetRateLimit is a free data retrieval call binding the contract method 0x0b0aee69.
//
// Solidity: function getRateLimit(address token) view returns(uint256)
func (_Escrow *EscrowSession) GetRateLimit(token common.Address) (*big.Int, error) {
	return _Escrow.Contract.GetRateLimit(&_Escrow.CallOpts, token)
}

// GetRateLimit is a free data retrieval call binding the contract method 0x0b0aee69.
//
// Solidity: function getRateLimit(address token) view returns(uint256)
func (_Escrow *EscrowCallerSession) GetRateLimit(token common.Address) (*big.Int, error) {
	return _Escrow.Contract.GetRateLimit(&_Escrow.CallOpts, token)
}

// Ics20 is a free data retrieval call binding the contract method 0x9a61e8e1.
//
// Solidity: function ics20() view returns(address)
func (_Escrow *EscrowCaller) Ics20(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Escrow.contract.Call(opts, &out, "ics20")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Ics20 is a free data retrieval call binding the contract method 0x9a61e8e1.
//
// Solidity: function ics20() view returns(address)
func (_Escrow *EscrowSession) Ics20() (common.Address, error) {
	return _Escrow.Contract.Ics20(&_Escrow.CallOpts)
}

// Ics20 is a free data retrieval call binding the contract method 0x9a61e8e1.
//
// Solidity: function ics20() view returns(address)
func (_Escrow *EscrowCallerSession) Ics20() (common.Address, error) {
	return _Escrow.Contract.Ics20(&_Escrow.CallOpts)
}

// IsConsumingScheduledOp is a free data retrieval call binding the contract method 0x8fb36037.
//
// Solidity: function isConsumingScheduledOp() view returns(bytes4)
func (_Escrow *EscrowCaller) IsConsumingScheduledOp(opts *bind.CallOpts) ([4]byte, error) {
	var out []interface{}
	err := _Escrow.contract.Call(opts, &out, "isConsumingScheduledOp")

	if err != nil {
		return *new([4]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([4]byte)).(*[4]byte)

	return out0, err

}

// IsConsumingScheduledOp is a free data retrieval call binding the contract method 0x8fb36037.
//
// Solidity: function isConsumingScheduledOp() view returns(bytes4)
func (_Escrow *EscrowSession) IsConsumingScheduledOp() ([4]byte, error) {
	return _Escrow.Contract.IsConsumingScheduledOp(&_Escrow.CallOpts)
}

// IsConsumingScheduledOp is a free data retrieval call binding the contract method 0x8fb36037.
//
// Solidity: function isConsumingScheduledOp() view returns(bytes4)
func (_Escrow *EscrowCallerSession) IsConsumingScheduledOp() ([4]byte, error) {
	return _Escrow.Contract.IsConsumingScheduledOp(&_Escrow.CallOpts)
}

// Initialize is a paid mutator transaction binding the contract method 0x485cc955.
//
// Solidity: function initialize(address ics20_, address authority) returns()
func (_Escrow *EscrowTransactor) Initialize(opts *bind.TransactOpts, ics20_ common.Address, authority common.Address) (*types.Transaction, error) {
	return _Escrow.contract.Transact(opts, "initialize", ics20_, authority)
}

// Initialize is a paid mutator transaction binding the contract method 0x485cc955.
//
// Solidity: function initialize(address ics20_, address authority) returns()
func (_Escrow *EscrowSession) Initialize(ics20_ common.Address, authority common.Address) (*types.Transaction, error) {
	return _Escrow.Contract.Initialize(&_Escrow.TransactOpts, ics20_, authority)
}

// Initialize is a paid mutator transaction binding the contract method 0x485cc955.
//
// Solidity: function initialize(address ics20_, address authority) returns()
func (_Escrow *EscrowTransactorSession) Initialize(ics20_ common.Address, authority common.Address) (*types.Transaction, error) {
	return _Escrow.Contract.Initialize(&_Escrow.TransactOpts, ics20_, authority)
}

// InitializeV2 is a paid mutator transaction binding the contract method 0x5cd8a76b.
//
// Solidity: function initializeV2() returns()
func (_Escrow *EscrowTransactor) InitializeV2(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Escrow.contract.Transact(opts, "initializeV2")
}

// InitializeV2 is a paid mutator transaction binding the contract method 0x5cd8a76b.
//
// Solidity: function initializeV2() returns()
func (_Escrow *EscrowSession) InitializeV2() (*types.Transaction, error) {
	return _Escrow.Contract.InitializeV2(&_Escrow.TransactOpts)
}

// InitializeV2 is a paid mutator transaction binding the contract method 0x5cd8a76b.
//
// Solidity: function initializeV2() returns()
func (_Escrow *EscrowTransactorSession) InitializeV2() (*types.Transaction, error) {
	return _Escrow.Contract.InitializeV2(&_Escrow.TransactOpts)
}

// RecvCallback is a paid mutator transaction binding the contract method 0xb4f22eb7.
//
// Solidity: function recvCallback(address token, address , uint256 amount) returns()
func (_Escrow *EscrowTransactor) RecvCallback(opts *bind.TransactOpts, token common.Address, arg1 common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Escrow.contract.Transact(opts, "recvCallback", token, arg1, amount)
}

// RecvCallback is a paid mutator transaction binding the contract method 0xb4f22eb7.
//
// Solidity: function recvCallback(address token, address , uint256 amount) returns()
func (_Escrow *EscrowSession) RecvCallback(token common.Address, arg1 common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Escrow.Contract.RecvCallback(&_Escrow.TransactOpts, token, arg1, amount)
}

// RecvCallback is a paid mutator transaction binding the contract method 0xb4f22eb7.
//
// Solidity: function recvCallback(address token, address , uint256 amount) returns()
func (_Escrow *EscrowTransactorSession) RecvCallback(token common.Address, arg1 common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Escrow.Contract.RecvCallback(&_Escrow.TransactOpts, token, arg1, amount)
}

// Send is a paid mutator transaction binding the contract method 0x0779afe6.
//
// Solidity: function send(address token, address to, uint256 amount) returns()
func (_Escrow *EscrowTransactor) Send(opts *bind.TransactOpts, token common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Escrow.contract.Transact(opts, "send", token, to, amount)
}

// Send is a paid mutator transaction binding the contract method 0x0779afe6.
//
// Solidity: function send(address token, address to, uint256 amount) returns()
func (_Escrow *EscrowSession) Send(token common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Escrow.Contract.Send(&_Escrow.TransactOpts, token, to, amount)
}

// Send is a paid mutator transaction binding the contract method 0x0779afe6.
//
// Solidity: function send(address token, address to, uint256 amount) returns()
func (_Escrow *EscrowTransactorSession) Send(token common.Address, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _Escrow.Contract.Send(&_Escrow.TransactOpts, token, to, amount)
}

// SetAuthority is a paid mutator transaction binding the contract method 0x7a9e5e4b.
//
// Solidity: function setAuthority(address newAuthority) returns()
func (_Escrow *EscrowTransactor) SetAuthority(opts *bind.TransactOpts, newAuthority common.Address) (*types.Transaction, error) {
	return _Escrow.contract.Transact(opts, "setAuthority", newAuthority)
}

// SetAuthority is a paid mutator transaction binding the contract method 0x7a9e5e4b.
//
// Solidity: function setAuthority(address newAuthority) returns()
func (_Escrow *EscrowSession) SetAuthority(newAuthority common.Address) (*types.Transaction, error) {
	return _Escrow.Contract.SetAuthority(&_Escrow.TransactOpts, newAuthority)
}

// SetAuthority is a paid mutator transaction binding the contract method 0x7a9e5e4b.
//
// Solidity: function setAuthority(address newAuthority) returns()
func (_Escrow *EscrowTransactorSession) SetAuthority(newAuthority common.Address) (*types.Transaction, error) {
	return _Escrow.Contract.SetAuthority(&_Escrow.TransactOpts, newAuthority)
}

// SetRateLimit is a paid mutator transaction binding the contract method 0xd34a3fd9.
//
// Solidity: function setRateLimit(address token, uint256 rateLimit) returns()
func (_Escrow *EscrowTransactor) SetRateLimit(opts *bind.TransactOpts, token common.Address, rateLimit *big.Int) (*types.Transaction, error) {
	return _Escrow.contract.Transact(opts, "setRateLimit", token, rateLimit)
}

// SetRateLimit is a paid mutator transaction binding the contract method 0xd34a3fd9.
//
// Solidity: function setRateLimit(address token, uint256 rateLimit) returns()
func (_Escrow *EscrowSession) SetRateLimit(token common.Address, rateLimit *big.Int) (*types.Transaction, error) {
	return _Escrow.Contract.SetRateLimit(&_Escrow.TransactOpts, token, rateLimit)
}

// SetRateLimit is a paid mutator transaction binding the contract method 0xd34a3fd9.
//
// Solidity: function setRateLimit(address token, uint256 rateLimit) returns()
func (_Escrow *EscrowTransactorSession) SetRateLimit(token common.Address, rateLimit *big.Int) (*types.Transaction, error) {
	return _Escrow.Contract.SetRateLimit(&_Escrow.TransactOpts, token, rateLimit)
}

// EscrowAuthorityUpdatedIterator is returned from FilterAuthorityUpdated and is used to iterate over the raw logs and unpacked data for AuthorityUpdated events raised by the Escrow contract.
type EscrowAuthorityUpdatedIterator struct {
	Event *EscrowAuthorityUpdated // Event containing the contract specifics and raw log

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
func (it *EscrowAuthorityUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EscrowAuthorityUpdated)
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
		it.Event = new(EscrowAuthorityUpdated)
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
func (it *EscrowAuthorityUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EscrowAuthorityUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EscrowAuthorityUpdated represents a AuthorityUpdated event raised by the Escrow contract.
type EscrowAuthorityUpdated struct {
	Authority common.Address
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterAuthorityUpdated is a free log retrieval operation binding the contract event 0x2f658b440c35314f52658ea8a740e05b284cdc84dc9ae01e891f21b8933e7cad.
//
// Solidity: event AuthorityUpdated(address authority)
func (_Escrow *EscrowFilterer) FilterAuthorityUpdated(opts *bind.FilterOpts) (*EscrowAuthorityUpdatedIterator, error) {

	logs, sub, err := _Escrow.contract.FilterLogs(opts, "AuthorityUpdated")
	if err != nil {
		return nil, err
	}
	return &EscrowAuthorityUpdatedIterator{contract: _Escrow.contract, event: "AuthorityUpdated", logs: logs, sub: sub}, nil
}

// WatchAuthorityUpdated is a free log subscription operation binding the contract event 0x2f658b440c35314f52658ea8a740e05b284cdc84dc9ae01e891f21b8933e7cad.
//
// Solidity: event AuthorityUpdated(address authority)
func (_Escrow *EscrowFilterer) WatchAuthorityUpdated(opts *bind.WatchOpts, sink chan<- *EscrowAuthorityUpdated) (event.Subscription, error) {

	logs, sub, err := _Escrow.contract.WatchLogs(opts, "AuthorityUpdated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EscrowAuthorityUpdated)
				if err := _Escrow.contract.UnpackLog(event, "AuthorityUpdated", log); err != nil {
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

// ParseAuthorityUpdated is a log parse operation binding the contract event 0x2f658b440c35314f52658ea8a740e05b284cdc84dc9ae01e891f21b8933e7cad.
//
// Solidity: event AuthorityUpdated(address authority)
func (_Escrow *EscrowFilterer) ParseAuthorityUpdated(log types.Log) (*EscrowAuthorityUpdated, error) {
	event := new(EscrowAuthorityUpdated)
	if err := _Escrow.contract.UnpackLog(event, "AuthorityUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// EscrowInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the Escrow contract.
type EscrowInitializedIterator struct {
	Event *EscrowInitialized // Event containing the contract specifics and raw log

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
func (it *EscrowInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(EscrowInitialized)
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
		it.Event = new(EscrowInitialized)
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
func (it *EscrowInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *EscrowInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// EscrowInitialized represents a Initialized event raised by the Escrow contract.
type EscrowInitialized struct {
	Version uint64
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_Escrow *EscrowFilterer) FilterInitialized(opts *bind.FilterOpts) (*EscrowInitializedIterator, error) {

	logs, sub, err := _Escrow.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &EscrowInitializedIterator{contract: _Escrow.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_Escrow *EscrowFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *EscrowInitialized) (event.Subscription, error) {

	logs, sub, err := _Escrow.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(EscrowInitialized)
				if err := _Escrow.contract.UnpackLog(event, "Initialized", log); err != nil {
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

// ParseInitialized is a log parse operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_Escrow *EscrowFilterer) ParseInitialized(log types.Log) (*EscrowInitialized, error) {
	event := new(EscrowInitialized)
	if err := _Escrow.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
