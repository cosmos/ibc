// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package accessmanager

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

// AccessManagerMetaData contains all meta data concerning the AccessManager contract.
var AccessManagerMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"initialAdmin\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"ADMIN_ROLE\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"PUBLIC_ROLE\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"canCall\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"selector\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"immediate\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"delay\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"cancel\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"consumeScheduledOp\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"execute\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"expiration\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAccess\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"since\",\"type\":\"uint48\",\"internalType\":\"uint48\"},{\"name\":\"currentDelay\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"pendingDelay\",\"type\":\"uint32\",\"internalType\":\"uint32\"},{\"name\":\"effect\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNonce\",\"inputs\":[{\"name\":\"id\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRoleAdmin\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRoleGrantDelay\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRoleGuardian\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getSchedule\",\"inputs\":[{\"name\":\"id\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTargetAdminDelay\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getTargetFunctionRole\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"selector\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"grantRole\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"executionDelay\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"hasRole\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"isMember\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"executionDelay\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"hashOperation\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isTargetClosed\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"labelRole\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"label\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"minSetback\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"multicall\",\"inputs\":[{\"name\":\"data\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[{\"name\":\"results\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceRole\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"callerConfirmation\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"revokeRole\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"schedule\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"},{\"name\":\"when\",\"type\":\"uint48\",\"internalType\":\"uint48\"}],\"outputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"nonce\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setGrantDelay\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"newDelay\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setRoleAdmin\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"admin\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setRoleGuardian\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"guardian\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setTargetAdminDelay\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newDelay\",\"type\":\"uint32\",\"internalType\":\"uint32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setTargetClosed\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"closed\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"setTargetFunctionRole\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"selectors\",\"type\":\"bytes4[]\",\"internalType\":\"bytes4[]\"},{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateAuthority\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"newAuthority\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"OperationCanceled\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"nonce\",\"type\":\"uint32\",\"indexed\":true,\"internalType\":\"uint32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OperationExecuted\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"nonce\",\"type\":\"uint32\",\"indexed\":true,\"internalType\":\"uint32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OperationScheduled\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"nonce\",\"type\":\"uint32\",\"indexed\":true,\"internalType\":\"uint32\"},{\"name\":\"schedule\",\"type\":\"uint48\",\"indexed\":false,\"internalType\":\"uint48\"},{\"name\":\"caller\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"target\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleAdminChanged\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"},{\"name\":\"admin\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleGrantDelayChanged\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"},{\"name\":\"delay\",\"type\":\"uint32\",\"indexed\":false,\"internalType\":\"uint32\"},{\"name\":\"since\",\"type\":\"uint48\",\"indexed\":false,\"internalType\":\"uint48\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleGranted\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"delay\",\"type\":\"uint32\",\"indexed\":false,\"internalType\":\"uint32\"},{\"name\":\"since\",\"type\":\"uint48\",\"indexed\":false,\"internalType\":\"uint48\"},{\"name\":\"newMember\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleGuardianChanged\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"},{\"name\":\"guardian\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleLabel\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"},{\"name\":\"label\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleRevoked\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"},{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"TargetAdminDelayUpdated\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"delay\",\"type\":\"uint32\",\"indexed\":false,\"internalType\":\"uint32\"},{\"name\":\"since\",\"type\":\"uint48\",\"indexed\":false,\"internalType\":\"uint48\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"TargetClosed\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"closed\",\"type\":\"bool\",\"indexed\":false,\"internalType\":\"bool\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"TargetFunctionRoleUpdated\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"selector\",\"type\":\"bytes4\",\"indexed\":false,\"internalType\":\"bytes4\"},{\"name\":\"roleId\",\"type\":\"uint64\",\"indexed\":true,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AccessManagerAlreadyScheduled\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"AccessManagerBadConfirmation\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AccessManagerExpired\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"AccessManagerInvalidInitialAdmin\",\"inputs\":[{\"name\":\"initialAdmin\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"AccessManagerLockedRole\",\"inputs\":[{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"AccessManagerNotReady\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"AccessManagerNotScheduled\",\"inputs\":[{\"name\":\"operationId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"AccessManagerUnauthorizedAccount\",\"inputs\":[{\"name\":\"msgsender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"roleId\",\"type\":\"uint64\",\"internalType\":\"uint64\"}]},{\"type\":\"error\",\"name\":\"AccessManagerUnauthorizedCall\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"selector\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}]},{\"type\":\"error\",\"name\":\"AccessManagerUnauthorizedCancel\",\"inputs\":[{\"name\":\"msgsender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"selector\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}]},{\"type\":\"error\",\"name\":\"AccessManagerUnauthorizedConsume\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"AddressEmptyCode\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientBalance\",\"inputs\":[{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"SafeCastOverflowedUintDowncast\",\"inputs\":[{\"name\":\"bits\",\"type\":\"uint8\",\"internalType\":\"uint8\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
	Bin: "0x6080604052346102b757604051601f6130e738819003918201601f19168301916001600160401b0383118484101761015a578084926020946040528339810103126102b757516001600160a01b038116908190036102b75780156102a4575f8181525f5160206130a75f395f51905f52602052604090205465ffffffffffff161580156101825765ffffffffffff610096426102bb565b1665ffffffffffff811161016e578060405191604083019383851060018060401b0386111761015a5760409485529183525f60208085018281528783525f5160206130a75f395f51905f529091529481209351845495516001600160a01b031990961665ffffffffffff919091161760309590951b600160301b600160a01b0316949094179092555f5160206130c75f395f51905f52916060915b65ffffffffffff604051928684521660208301526040820152a3604051612dbc90816102eb8239f35b634e487b7160e01b5f52604160045260245ffd5b634e487b7160e01b5f52601160045260245ffd5b5f8281525f5160206130a75f395f51905f526020526040902054906101a6426102bb565b63ffffffff8360301c169265ffffffffffff808260701c1692168211155f146102925750505b63ffffffff8216801561028b5763ffffffff811161016e575b65ffffffffffff63ffffffff6101fa426102bb565b921691160165ffffffffffff811161016e575f8481525f5160206130a75f395f51905f52602090815260408083208054600160301b600160a01b0319169185901b6dffffffffffff0000000000000000169690921b67ffffffff00000000169590951760301b600160301b600160a01b0316949094179093555f5160206130c75f395f51905f529160609190610131565b505f6101e5565b63ffffffff9060501c169250506101cc565b630409d6d160e11b5f525f60045260245ffd5b5f80fd5b65ffffffffffff81116102d35765ffffffffffff1690565b6306dfcc6560e41b5f52603060045260245260445ffdfe60806040526004361015610011575f80fd5b5f5f3560e01c806308d6122d14611a305780630b0a93ba146119e957806312be8727146119c6578063167bd3951461190957806318ff183c146118485780631cff79cd1461169757806325c471a01461128c5780633078f1141461123157806330cae187146111715780633adc277a146111425780633ca7c02a1461111f5780634136a33c146110ec5780634665096d146110ce5780634c1da1e21461109c5780635296295214610fc8578063530dd45614610f715780636d5115bd14610eeb57806375b238fc14610ecf578063853551b814610dfa57806394c7d7ee14610c98578063a166aa8914610c42578063a64d95ce14610afb578063abd9bd2a14610ad6578063ac9650d8146108eb578063b70096131461088e578063b7d2b1621461085b578063cc1b6c811461083d578063d1f856ee146107f8578063d22b5989146106f5578063d6bb62c6146104a7578063f801a698146101f25763fe0776f51461017a575f80fd5b346101ef5760406003193601126101ef57610193611bc7565b61019b611b73565b903373ffffffffffffffffffffffffffffffffffffffff8316036101c757906101c3916124da565b5080f35b6004837f5f159e63000000000000000000000000000000000000000000000000000000008152fd5b80fd5b50346101ef5760606003193601126101ef5761020c611b50565b9060243567ffffffffffffffff81116104a35761022d903690600401611bf5565b919060443565ffffffffffff811680910361049f5761024e848387336121a5565b905061026a63ffffffff61026142612d3e565b921680926120a1565b90158015610484575b61040e579065ffffffffffff809216908180821191180218169061029984828733611e6a565b93848452600260205265ffffffffffff60408520541680151590816103fd575b506103d15760409584936103c287947f82a2da5dee54ea8021c6545b4444620291e07ee83be6dd57edb175062715f3b494868b99526002602052600163ffffffff8a8a205460301c160163ffffffff81169989898c9b52600260205281812065ffffffffffff88167fffffffffffffffffffffffffffffffffffffffffffffffffffff000000000000825416179055898152600260205220907fffffffffffffffffffffffffffffffffffffffffffff00000000ffffffffffff69ffffffff00000000000083549260301b16911617905573ffffffffffffffffffffffffffffffffffffffff8b519586958652336020870152168b850152608060608501526080840191611e4a565b0390a382519182526020820152f35b602484867f813e9459000000000000000000000000000000000000000000000000000000008252600452fd5b6104079150612484565b155f6102b9565b6064847fffffffff000000000000000000000000000000000000000000000000000000008873ffffffffffffffffffffffffffffffffffffffff6104528a896121f9565b917f81c6f24b000000000000000000000000000000000000000000000000000000008552336004521660245216604452fd5b508115158015610273575065ffffffffffff81168210610273565b8280fd5b5080fd5b50346101ef576104cf906104ba36611c87565b6104c781839497936121f9565b928685611e6a565b91828452600260205265ffffffffffff604085205416155f1461051857602484847f60a299b0000000000000000000000000000000000000000000000000000000008252600452fd5b73ffffffffffffffffffffffffffffffffffffffff16903382036105b0575b50506020925080825260028352604082207fffffffffffffffffffffffffffffffffffffffffffffffffffff00000000000081541690558082526002835263ffffffff604083205460301c1680917fbd9ac67a6e2f6463b80927326310338bcbb4bdb7936ce1365ea3e01067e7b9f76040519480a38152f35b65ffffffffffff946105c2335f611d7d565b505096169586151596876106c1575b509073ffffffffffffffffffffffffffffffffffffffff91501694858552846020527fffffffff00000000000000000000000000000000000000000000000000000000604086209216918286526020526106653361066067ffffffffffffffff60408920541667ffffffffffffffff165f52600160205267ffffffffffffffff600160405f20015460401c1690565b61204a565b50901590816106b8575b5015610537576084925084604051927f3fe2751c000000000000000000000000000000000000000000000000000000008452336004850152602484015260448301526064820152fd5b9050155f61066f565b73ffffffffffffffffffffffffffffffffffffffff9291975065ffffffffffff6106ea42612d3e565b1610159690916105d1565b50346101ef5760406003193601126101ef5761070f611b50565b7fa56b76017453f399ec2327ba00375dbfb1fd070ff854341ad6191e6a2e2de19c73ffffffffffffffffffffffffffffffffffffffff61074d611c74565b926107566120ec565b169182845283602052610780816dffffffffffffffffffffffffffff600160408820015416612ca6565b9190848652856020526dffffffffffffffffffffffffffff6001604088200191167fffffffffffffffffffffffffffffffffffff00000000000000000000000000008254161790556107f26040519283928390929165ffffffffffff60209163ffffffff604085019616845216910152565b0390a280f35b50346101ef5760406003193601126101ef57610823610815611bc7565b61081d611b73565b9061204a565b60408051921515835263ffffffff91909116602083015290f35b50346101ef57806003193601126101ef576020604051620697808152f35b50346101ef5760406003193601126101ef576101c3610878611bc7565b610880611b73565b906108896120ec565b6124da565b50346101ef5760606003193601126101ef576108a8611b50565b6108b0611b73565b604435917fffffffff00000000000000000000000000000000000000000000000000000000831683036108e7576108239350611f1b565b8380fd5b50346101ef5760206003193601126101ef5760043567ffffffffffffffff81116104a35761091d903690600401611b96565b90602060405161092d8282611d2d565b84815281810191601f19810136843761094585611ec2565b936109536040519586611d2d565b858552601f1961096287611ec2565b01875b818110610ac757505086907fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe181360301915b87811015610a31578060051b82013583811215610a2d5782019081359167ffffffffffffffff8311610a295785018236038113610a295782610a07610a0d928d898c6001986040519687958487013784018281018481528e519283915e010190815203601f198101835282611d2d565b306124b8565b610a17828a611eda565b52610a228189611eda565b5001610997565b8a80fd5b8980fd5b88848860405191808301818452825180915260408401918060408360051b870101940192865b838810610a645786860387f35b9091929394838080837fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc08b60019603018752601f19601f838c518051918291828752018686015e8885828601015201160101970193019701969093929193610a57565b60608782018501528301610965565b50346101ef576020610af3610aea36611c87565b92919091611e6a565b604051908152f35b50346101ef5760406003193601126101ef57610b15611bc7565b67ffffffffffffffff610b26611c74565b91610b2f6120ec565b169067ffffffffffffffff8214610c16577ffeb69018ee8b8fd50ea86348f1267d07673379f72cffdeccec63853ee8ce8b48908284526001602052610b8e816dffffffffffffffffffffffffffff600160408820015460801c16612ca6565b9190848652600160205260016040872001907fffff0000000000000000000000000000ffffffffffffffffffffffffffffffff7dffffffffffffffffffffffffffff0000000000000000000000000000000083549260801b1691161790556107f26040519283928390929165ffffffffffff60209163ffffffff604085019616845216910152565b602483837f1871a90c000000000000000000000000000000000000000000000000000000008252600452fd5b50346101ef5760206003193601126101ef576020610c8e610c61611b50565b73ffffffffffffffffffffffffffffffffffffffff165f525f60205260ff600160405f20015460701c1690565b6040519015158152f35b50346101ef57610ca736611c23565b916040517f8fb36037000000000000000000000000000000000000000000000000000000008152602081600481335afa908115610def578591610d70575b507fffffffff000000000000000000000000000000000000000000000000000000007f8fb3603700000000000000000000000000000000000000000000000000000000911603610d445791610d3f916101c3933390611e6a565b612227565b6024847f320ff74800000000000000000000000000000000000000000000000000000000815233600452fd5b90506020813d602011610de7575b81610d8b60209383611d2d565b81010312610de357517fffffffff0000000000000000000000000000000000000000000000000000000081168103610de3577fffffffff00000000000000000000000000000000000000000000000000000000610ce5565b8480fd5b3d9150610d7e565b6040513d87823e3d90fd5b50346101ef5760406003193601126101ef57610e14611bc7565b6024359067ffffffffffffffff821161049f57610e3e67ffffffffffffffff923690600401611bf5565b929091610e496120ec565b169182158015610ebe575b610e9257907f1256f5b5ecb89caec12db449738f2fbcd1ba5806cf38f35413f4e5c15bf6a450916107f2604051928392602084526020840191611e4a565b602484847f1871a90c000000000000000000000000000000000000000000000000000000008252600452fd5b5067ffffffffffffffff8314610e54565b50346101ef57806003193601126101ef57602090604051908152f35b50346101ef5760406003193601126101ef57610f05611b50565b602435907fffffffff00000000000000000000000000000000000000000000000000000000821680920361049f5760209267ffffffffffffffff9273ffffffffffffffffffffffffffffffffffffffff6040931682528185528282209082528452205416604051908152f35b50346101ef5760206003193601126101ef576020610fb6610f90611bc7565b67ffffffffffffffff165f52600160205267ffffffffffffffff600160405f2001541690565b67ffffffffffffffff60405191168152f35b50346101ef5760406003193601126101ef57610fe2611bc7565b67ffffffffffffffff610ff3611bde565b91610ffc6120ec565b16908115801561108b575b610c165767ffffffffffffffff9082845260016020526001604085200180547fffffffffffffffffffffffffffffffff0000000000000000ffffffffffffffff6fffffffffffffffff00000000000000008460401b16911617905516907f7a8059630b897b5de4c08ade69f8b90c3ead1f8596d62d10b6c4d14a0afb4ae28380a380f35b5067ffffffffffffffff8214611007565b50346101ef5760206003193601126101ef5760206110c06110bb611b50565b611e0e565b63ffffffff60405191168152f35b50346101ef57806003193601126101ef57602060405162093a808152f35b50346101ef5760206003193601126101ef5763ffffffff6040602092600435815260028452205460301c16604051908152f35b50346101ef57806003193601126101ef57602060405167ffffffffffffffff8152f35b50346101ef5760206003193601126101ef576020611161600435611de4565b65ffffffffffff60405191168152f35b50346101ef5760406003193601126101ef5761118b611bc7565b67ffffffffffffffff61119c611bde565b916111a56120ec565b169081158015611220575b610c165767ffffffffffffffff908284526001602052600160408520018282167fffffffffffffffffffffffffffffffffffffffffffffffff000000000000000082541617905516907f1fd6dd7631312dfac2205b52913f99de03b4d7e381d5d27d3dbfe0713e6e63408380a380f35b5067ffffffffffffffff82146111b0565b50346101ef5760406003193601126101ef57608063ffffffff65ffffffffffff8161126b61125d611bc7565b611265611b73565b90611d7d565b93929590918560405197168752166020860152166040840152166060820152f35b50346101ef5760606003193601126101ef576112a6611bc7565b6112ae611b73565b906044359163ffffffff83168093036108e7576112c96120ec565b67ffffffffffffffff6112db83611cf4565b92169167ffffffffffffffff831461166b5782855260016020526040852073ffffffffffffffffffffffffffffffffffffffff83165f5260205265ffffffffffff60405f2054161590815f14611493576113459063ffffffff61133d42612d3e565b9116906120a1565b6040516040810181811067ffffffffffffffff821117611466579265ffffffffffff73ffffffffffffffffffffffffffffffffffffffff9361144f7ff98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf97946dffffffffffffffffffffffffffff8b60408e60609b82528787168552602085019283528d81526001602052208989165f52602052858060405f20945116167fffffffffffffffffffffffffffffffffffffffffffffffffffff00000000000084541617835551167fffffffffffffffffffffffff0000000000000000000000000000ffffffffffff73ffffffffffffffffffffffffffff00000000000083549260301b169116179055565b60405198895216602088015260408701521693a380f35b6024887f4e487b710000000000000000000000000000000000000000000000000000000081526041600452fd5b5082855260016020526040852073ffffffffffffffffffffffffffffffffffffffff83165f526020526114dc6dffffffffffffffffffffffffffff60405f205460301c16612447565b50508463ffffffff82168181115f1461160d570363ffffffff81116115e0579260609265ffffffffffff73ffffffffffffffffffffffffffffffffffffffff936115db67ffffffff0000000061156263ffffffff7ff98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf9a5b1661155d42612d3e565b6120a1565b9260201b166dffffffffffff00000000000000008360401b16178a17898c52600160205260408c208787165f5260205260405f20907fffffffffffffffffffffffff0000000000000000000000000000ffffffffffff73ffffffffffffffffffffffffffff00000000000083549260301b169116179055565b61144f565b6024877f4e487b710000000000000000000000000000000000000000000000000000000081526011600452fd5b50507ff98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf9260609265ffffffffffff73ffffffffffffffffffffffffffffffffffffffff936115db67ffffffff0000000061156263ffffffff8d611553565b602485847f1871a90c000000000000000000000000000000000000000000000000000000008252600452fd5b506116a136611c23565b90916116af828483336121a5565b9390158061183a575b6117f6576116c883828433611e6a565b63ffffffff869516158015906117dd575b6117cb575b50600354926117376116f082846121f9565b849073ffffffffffffffffffffffffffffffffffffffff7fffffffff0000000000000000000000000000000000000000000000000000000092165f521660205260405f2090565b60035567ffffffffffffffff811161179e5760405191611761601f8301601f191660200184611d2d565b818352368282011161179a5795602082849382998361178898970137830101523491612358565b5060035563ffffffff60405191168152f35b8680fd5b6024867f4e487b710000000000000000000000000000000000000000000000000000000081526041600452fd5b6117d6919450612227565b925f6116de565b5065ffffffffffff6117ee82611de4565b1615156116d9565b849173ffffffffffffffffffffffffffffffffffffffff6104526064957fffffffff00000000000000000000000000000000000000000000000000000000946121f9565b5063ffffffff8416156116b8565b503461190557604060031936011261190557611862611b50565b73ffffffffffffffffffffffffffffffffffffffff61187f611b73565b916118886120ec565b1690813b156119055773ffffffffffffffffffffffffffffffffffffffff60245f928360405195869485937f7a9e5e4b0000000000000000000000000000000000000000000000000000000085521660048401525af180156118fa576118ec575080f35b6118f891505f90611d2d565b005b6040513d5f823e3d90fd5b5f80fd5b3461190557604060031936011261190557611922611b50565b6024359081151580920361190557602073ffffffffffffffffffffffffffffffffffffffff7f90d4e7bb7e5d933792b3562e1741306f8be94837e1348dacef9b6f1df56eb138926119716120ec565b1692835f525f8252600160405f200180547fffffffffffffffffffffffffffffffffff00ffffffffffffffffffffffffffff6eff00000000000000000000000000008460701b169116179055604051908152a2005b346119055760206003193601126119055760206110c06119e4611bc7565b611cf4565b34611905576020600319360112611905576020610fb6611a07611bc7565b67ffffffffffffffff165f52600160205267ffffffffffffffff600160405f20015460401c1690565b3461190557606060031936011261190557611a49611b50565b60243567ffffffffffffffff811161190557611a69903690600401611b96565b91906044359067ffffffffffffffff821680920361190557611a8c9392936120ec565b73ffffffffffffffffffffffffffffffffffffffff5f9416935b838110156118f8578060051b820135907fffffffff0000000000000000000000000000000000000000000000000000000082168092036119055783867f9ea6790c7dadfd01c9f8b9762b3682607af2c7e79e05a9f9fdf5580dde9491516020600195835f525f825260405f20815f52825260405f20857fffffffffffffffffffffffffffffffffffffffffffffffff0000000000000000825416179055604051908152a301611aa6565b6004359073ffffffffffffffffffffffffffffffffffffffff8216820361190557565b6024359073ffffffffffffffffffffffffffffffffffffffff8216820361190557565b9181601f840112156119055782359167ffffffffffffffff8311611905576020808501948460051b01011161190557565b6004359067ffffffffffffffff8216820361190557565b6024359067ffffffffffffffff8216820361190557565b9181601f840112156119055782359167ffffffffffffffff8311611905576020838186019501011161190557565b9060406003198301126119055760043573ffffffffffffffffffffffffffffffffffffffff8116810361190557916024359067ffffffffffffffff821161190557611c7091600401611bf5565b9091565b6024359063ffffffff8216820361190557565b60606003198201126119055760043573ffffffffffffffffffffffffffffffffffffffff81168103611905579160243573ffffffffffffffffffffffffffffffffffffffff8116810361190557916044359067ffffffffffffffff821161190557611c7091600401611bf5565b67ffffffffffffffff165f526001602052611d286dffffffffffffffffffffffffffff600160405f20015460801c16612447565b505090565b90601f601f19910116810190811067ffffffffffffffff821117611d5057604052565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52604160045260245ffd5b9067ffffffffffffffff65ffffffffffff9392165f52600160205273ffffffffffffffffffffffffffffffffffffffff60405f2091165f5260205260405f205490611dda6dffffffffffffffffffffffffffff8360301c16612447565b9490931693909291565b5f52600260205265ffffffffffff60405f205416611e0181612484565b15611e0b57505f90565b90565b73ffffffffffffffffffffffffffffffffffffffff165f525f602052611d286dffffffffffffffffffffffffffff600160405f20015416612447565b601f8260209493601f1993818652868601375f8582860101520116010190565b9290611eae73ffffffffffffffffffffffffffffffffffffffff93611ebc936040519586948160208701991689521660408501526060808501526080840191611e4a565b03601f198101835282611d2d565b51902090565b67ffffffffffffffff8111611d505760051b60200190565b8051821015611eee5760209160051b010190565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52603260045260245ffd5b919091611f4f8373ffffffffffffffffffffffffffffffffffffffff165f525f60205260ff600160405f20015460701c1690565b15611f5d575050505f905f90565b73ffffffffffffffffffffffffffffffffffffffff81163003611fcf5750611fc990600354929073ffffffffffffffffffffffffffffffffffffffff7fffffffff0000000000000000000000000000000000000000000000000000000092165f521660205260405f2090565b14905f90565b9073ffffffffffffffffffffffffffffffffffffffff61203093165f525f6020527fffffffff0000000000000000000000000000000000000000000000000000000060405f2091165f5260205267ffffffffffffffff60405f20541661204a565b9190156120435763ffffffff8216159190565b5f91508190565b67ffffffffffffffff818116036120645750506001905f90565b65ffffffffffff929161207691611d7d565b50509216801515908161208857509190565b905065ffffffffffff61209a42612d3e565b1610159190565b9065ffffffffffff8091169116019065ffffffffffff82116120bf57565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52601160045260245ffd5b6120f636336125bb565b90156120ff5750565b63ffffffff1661214e5767ffffffffffffffff61211b36612721565b5090507ff07e038f000000000000000000000000000000000000000000000000000000005f52336004521660245260445ffd5b6121a2604051602081019033825230604082015260608082015261219a81602060808201368152365f838301375f823683010152601f19601f360116010103601f198101835282611d2d565b519020612227565b50565b9092919073ffffffffffffffffffffffffffffffffffffffff841630036121d057611c7093506126ad565b91929060048410156121e657505050505f905f90565b611c70936121f3916121f9565b91611f1b565b9060041161190557357fffffffff000000000000000000000000000000000000000000000000000000001690565b5f81815260026020526040902054909190603081901c63ffffffff169065ffffffffffff168061227d57837f60a299b0000000000000000000000000000000000000000000000000000000005f5260045260245ffd5b65ffffffffffff61228d42612d3e565b168111156122c157837f18cb6b7a000000000000000000000000000000000000000000000000000000005f5260045260245ffd5b6122ce9093919293612484565b61232d578190805f52600260205260405f207fffffffffffffffffffffffffffffffffffffffffffffffffffff00000000000081541690557f76a2a46953689d4861a5d3f6ed883ad7e6af674a21f8e162707159fc9dde614d5f80a390565b7f78a5d6e4000000000000000000000000000000000000000000000000000000005f5260045260245ffd5b9180471061241857815f92916020849351920190855af18080612405575b15612385575050611e0b612c8d565b156123cc5773ffffffffffffffffffffffffffffffffffffffff907f9996b315000000000000000000000000000000000000000000000000000000005f521660045260245ffd5b3d156123dd576040513d5f823e3d90fd5b7fd6bda275000000000000000000000000000000000000000000000000000000005f5260045ffd5b503d1515806123765750813b1515612376565b477fcf479181000000000000000000000000000000000000000000000000000000005f5260045260245260445ffd5b61245042612d3e565b63ffffffff82169165ffffffffffff604082901c811692168211612478575090915f91508190565b60201c63ffffffff1692565b65ffffffffffff62093a8091160165ffffffffffff81116120bf5765ffffffffffff806124b042612d3e565b169116111590565b905f8091602081519101845af480806124055715612385575050611e0b612c8d565b67ffffffffffffffff169067ffffffffffffffff821461258f57815f52600160205260405f2073ffffffffffffffffffffffffffffffffffffffff82165f5260205265ffffffffffff60405f205416156125895773ffffffffffffffffffffffffffffffffffffffff90825f52600160205260405f208282165f526020525f604081205516907ff229baa593af28c41b1d16b748cd7688f0c83aaf92d4be41c44005defe84c1665f80a3600190565b50505f90565b507f1871a90c000000000000000000000000000000000000000000000000000000005f5260045260245ffd5b9060048110612624573073ffffffffffffffffffffffffffffffffffffffff83161461266c576125eb905f6129eb565b9290911580612635575b61262c576126029161204a565b90156126245763ffffffff808093169116908180821191180218169081159190565b50505f905f90565b5050505f905f90565b506126673073ffffffffffffffffffffffffffffffffffffffff165f525f60205260ff600160405f20015460701c1690565b6125f5565b905060041161190557600354305f90815280357fffffffff000000000000000000000000000000000000000000000000000000001660205260409020611fc9565b91906004821061262c573073ffffffffffffffffffffffffffffffffffffffff8416146126de57906125eb916129eb565b6126e892506121f9565b600354305f9081527fffffffff000000000000000000000000000000000000000000000000000000009092166020526040909120611fc9565b5f90600481106129e15780600411611905577fffffffff000000000000000000000000000000000000000000000000000000005f3516907f853551b800000000000000000000000000000000000000000000000000000000821480156129b8575b801561298f575b8015612966575b801561293d575b612931577f18ff183c0000000000000000000000000000000000000000000000000000000082148015612908575b80156128df575b6128a3577f25c471a0000000000000000000000000000000000000000000000000000000008214801561287a575b6128265750308252816020526040822090825260205267ffffffffffffffff6040822054169181929190565b90506024116101ef57806101ef575060043567ffffffffffffffff81168103611905576128739067ffffffffffffffff165f52600160205267ffffffffffffffff600160405f2001541690565b6001915f90565b507fb7d2b1620000000000000000000000000000000000000000000000000000000082146127fa565b9150506024116119055760043573ffffffffffffffffffffffffffffffffffffffff8116809103611905576128d790611e0e565b6001915f9190565b507f08d6122d0000000000000000000000000000000000000000000000000000000082146127cc565b507f167bd3950000000000000000000000000000000000000000000000000000000082146127c5565b5050506001905f905f90565b507fd22b5989000000000000000000000000000000000000000000000000000000008214612797565b507fa64d95ce000000000000000000000000000000000000000000000000000000008214612790565b507f52962952000000000000000000000000000000000000000000000000000000008214612789565b507f30cae187000000000000000000000000000000000000000000000000000000008214612782565b50505f905f905f90565b600482106129e1577fffffffff00000000000000000000000000000000000000000000000000000000612a1e83836121f9565b16917f853551b80000000000000000000000000000000000000000000000000000000083148015612c64575b8015612c3b575b8015612c12575b8015612be9575b612931577f18ff183c0000000000000000000000000000000000000000000000000000000083148015612bc0575b8015612b97575b612b62577f25c471a00000000000000000000000000000000000000000000000000000000083148015612b39575b612af0575050305f525f60205260405f20905f5260205267ffffffffffffffff60405f205416905f91905f90565b909150602411611905576004013567ffffffffffffffff81168103611905576128739067ffffffffffffffff165f52600160205267ffffffffffffffff600160405f2001541690565b507fb7d2b162000000000000000000000000000000000000000000000000000000008314612ac2565b909150602411611905576004013573ffffffffffffffffffffffffffffffffffffffff8116809103611905576128d790611e0e565b507f08d6122d000000000000000000000000000000000000000000000000000000008314612a94565b507f167bd395000000000000000000000000000000000000000000000000000000008314612a8d565b507fd22b5989000000000000000000000000000000000000000000000000000000008314612a5f565b507fa64d95ce000000000000000000000000000000000000000000000000000000008314612a58565b507f52962952000000000000000000000000000000000000000000000000000000008314612a51565b507f30cae187000000000000000000000000000000000000000000000000000000008314612a4a565b604051903d82523d5f602084013e60203d830101604052565b612cb763ffffffff91939293612447565b505092168063ffffffff84168181115f14612d24570363ffffffff81116120bf57612d0563ffffffff8067ffffffff00000000935b1680620697801181620697801802181661155d42612d3e565b9360201b166dffffffffffff00000000000000008460401b1617179190565b505067ffffffff00000000612d0563ffffffff805f612cec565b65ffffffffffff8111612d565765ffffffffffff1690565b7f6dfcc650000000000000000000000000000000000000000000000000000000005f52603060045260245260445ffdfea2646970667358221220ba24eba1277b41d77a8f22ee63cfafc84f90777053a193a324f7e2908f3df96a64736f6c634300081c0033a6eef7e35abe7026729641147f7915573c7e97b47efa546f5f6e3230263bcb49f98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf",
}

// AccessManagerABI is the input ABI used to generate the binding from.
// Deprecated: Use AccessManagerMetaData.ABI instead.
var AccessManagerABI = AccessManagerMetaData.ABI

// AccessManagerBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use AccessManagerMetaData.Bin instead.
var AccessManagerBin = AccessManagerMetaData.Bin

// DeployAccessManager deploys a new Ethereum contract, binding an instance of AccessManager to it.
func DeployAccessManager(auth *bind.TransactOpts, backend bind.ContractBackend, initialAdmin common.Address) (common.Address, *types.Transaction, *AccessManager, error) {
	parsed, err := AccessManagerMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(AccessManagerBin), backend, initialAdmin)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &AccessManager{AccessManagerCaller: AccessManagerCaller{contract: contract}, AccessManagerTransactor: AccessManagerTransactor{contract: contract}, AccessManagerFilterer: AccessManagerFilterer{contract: contract}}, nil
}

// AccessManager is an auto generated Go binding around an Ethereum contract.
type AccessManager struct {
	AccessManagerCaller     // Read-only binding to the contract
	AccessManagerTransactor // Write-only binding to the contract
	AccessManagerFilterer   // Log filterer for contract events
}

// AccessManagerCaller is an auto generated read-only Go binding around an Ethereum contract.
type AccessManagerCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// AccessManagerTransactor is an auto generated write-only Go binding around an Ethereum contract.
type AccessManagerTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// AccessManagerFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type AccessManagerFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// AccessManagerSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type AccessManagerSession struct {
	Contract     *AccessManager    // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// AccessManagerCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type AccessManagerCallerSession struct {
	Contract *AccessManagerCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts        // Call options to use throughout this session
}

// AccessManagerTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type AccessManagerTransactorSession struct {
	Contract     *AccessManagerTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// AccessManagerRaw is an auto generated low-level Go binding around an Ethereum contract.
type AccessManagerRaw struct {
	Contract *AccessManager // Generic contract binding to access the raw methods on
}

// AccessManagerCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type AccessManagerCallerRaw struct {
	Contract *AccessManagerCaller // Generic read-only contract binding to access the raw methods on
}

// AccessManagerTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type AccessManagerTransactorRaw struct {
	Contract *AccessManagerTransactor // Generic write-only contract binding to access the raw methods on
}

// NewAccessManager creates a new instance of AccessManager, bound to a specific deployed contract.
func NewAccessManager(address common.Address, backend bind.ContractBackend) (*AccessManager, error) {
	contract, err := bindAccessManager(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &AccessManager{AccessManagerCaller: AccessManagerCaller{contract: contract}, AccessManagerTransactor: AccessManagerTransactor{contract: contract}, AccessManagerFilterer: AccessManagerFilterer{contract: contract}}, nil
}

// NewAccessManagerCaller creates a new read-only instance of AccessManager, bound to a specific deployed contract.
func NewAccessManagerCaller(address common.Address, caller bind.ContractCaller) (*AccessManagerCaller, error) {
	contract, err := bindAccessManager(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &AccessManagerCaller{contract: contract}, nil
}

// NewAccessManagerTransactor creates a new write-only instance of AccessManager, bound to a specific deployed contract.
func NewAccessManagerTransactor(address common.Address, transactor bind.ContractTransactor) (*AccessManagerTransactor, error) {
	contract, err := bindAccessManager(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &AccessManagerTransactor{contract: contract}, nil
}

// NewAccessManagerFilterer creates a new log filterer instance of AccessManager, bound to a specific deployed contract.
func NewAccessManagerFilterer(address common.Address, filterer bind.ContractFilterer) (*AccessManagerFilterer, error) {
	contract, err := bindAccessManager(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &AccessManagerFilterer{contract: contract}, nil
}

// bindAccessManager binds a generic wrapper to an already deployed contract.
func bindAccessManager(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := AccessManagerMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_AccessManager *AccessManagerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _AccessManager.Contract.AccessManagerCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_AccessManager *AccessManagerRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _AccessManager.Contract.AccessManagerTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_AccessManager *AccessManagerRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _AccessManager.Contract.AccessManagerTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_AccessManager *AccessManagerCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _AccessManager.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_AccessManager *AccessManagerTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _AccessManager.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_AccessManager *AccessManagerTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _AccessManager.Contract.contract.Transact(opts, method, params...)
}

// ADMINROLE is a free data retrieval call binding the contract method 0x75b238fc.
//
// Solidity: function ADMIN_ROLE() view returns(uint64)
func (_AccessManager *AccessManagerCaller) ADMINROLE(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "ADMIN_ROLE")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// ADMINROLE is a free data retrieval call binding the contract method 0x75b238fc.
//
// Solidity: function ADMIN_ROLE() view returns(uint64)
func (_AccessManager *AccessManagerSession) ADMINROLE() (uint64, error) {
	return _AccessManager.Contract.ADMINROLE(&_AccessManager.CallOpts)
}

// ADMINROLE is a free data retrieval call binding the contract method 0x75b238fc.
//
// Solidity: function ADMIN_ROLE() view returns(uint64)
func (_AccessManager *AccessManagerCallerSession) ADMINROLE() (uint64, error) {
	return _AccessManager.Contract.ADMINROLE(&_AccessManager.CallOpts)
}

// PUBLICROLE is a free data retrieval call binding the contract method 0x3ca7c02a.
//
// Solidity: function PUBLIC_ROLE() view returns(uint64)
func (_AccessManager *AccessManagerCaller) PUBLICROLE(opts *bind.CallOpts) (uint64, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "PUBLIC_ROLE")

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// PUBLICROLE is a free data retrieval call binding the contract method 0x3ca7c02a.
//
// Solidity: function PUBLIC_ROLE() view returns(uint64)
func (_AccessManager *AccessManagerSession) PUBLICROLE() (uint64, error) {
	return _AccessManager.Contract.PUBLICROLE(&_AccessManager.CallOpts)
}

// PUBLICROLE is a free data retrieval call binding the contract method 0x3ca7c02a.
//
// Solidity: function PUBLIC_ROLE() view returns(uint64)
func (_AccessManager *AccessManagerCallerSession) PUBLICROLE() (uint64, error) {
	return _AccessManager.Contract.PUBLICROLE(&_AccessManager.CallOpts)
}

// CanCall is a free data retrieval call binding the contract method 0xb7009613.
//
// Solidity: function canCall(address caller, address target, bytes4 selector) view returns(bool immediate, uint32 delay)
func (_AccessManager *AccessManagerCaller) CanCall(opts *bind.CallOpts, caller common.Address, target common.Address, selector [4]byte) (struct {
	Immediate bool
	Delay     uint32
}, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "canCall", caller, target, selector)

	outstruct := new(struct {
		Immediate bool
		Delay     uint32
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Immediate = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.Delay = *abi.ConvertType(out[1], new(uint32)).(*uint32)

	return *outstruct, err

}

// CanCall is a free data retrieval call binding the contract method 0xb7009613.
//
// Solidity: function canCall(address caller, address target, bytes4 selector) view returns(bool immediate, uint32 delay)
func (_AccessManager *AccessManagerSession) CanCall(caller common.Address, target common.Address, selector [4]byte) (struct {
	Immediate bool
	Delay     uint32
}, error) {
	return _AccessManager.Contract.CanCall(&_AccessManager.CallOpts, caller, target, selector)
}

// CanCall is a free data retrieval call binding the contract method 0xb7009613.
//
// Solidity: function canCall(address caller, address target, bytes4 selector) view returns(bool immediate, uint32 delay)
func (_AccessManager *AccessManagerCallerSession) CanCall(caller common.Address, target common.Address, selector [4]byte) (struct {
	Immediate bool
	Delay     uint32
}, error) {
	return _AccessManager.Contract.CanCall(&_AccessManager.CallOpts, caller, target, selector)
}

// Expiration is a free data retrieval call binding the contract method 0x4665096d.
//
// Solidity: function expiration() view returns(uint32)
func (_AccessManager *AccessManagerCaller) Expiration(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "expiration")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// Expiration is a free data retrieval call binding the contract method 0x4665096d.
//
// Solidity: function expiration() view returns(uint32)
func (_AccessManager *AccessManagerSession) Expiration() (uint32, error) {
	return _AccessManager.Contract.Expiration(&_AccessManager.CallOpts)
}

// Expiration is a free data retrieval call binding the contract method 0x4665096d.
//
// Solidity: function expiration() view returns(uint32)
func (_AccessManager *AccessManagerCallerSession) Expiration() (uint32, error) {
	return _AccessManager.Contract.Expiration(&_AccessManager.CallOpts)
}

// GetAccess is a free data retrieval call binding the contract method 0x3078f114.
//
// Solidity: function getAccess(uint64 roleId, address account) view returns(uint48 since, uint32 currentDelay, uint32 pendingDelay, uint48 effect)
func (_AccessManager *AccessManagerCaller) GetAccess(opts *bind.CallOpts, roleId uint64, account common.Address) (struct {
	Since        *big.Int
	CurrentDelay uint32
	PendingDelay uint32
	Effect       *big.Int
}, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getAccess", roleId, account)

	outstruct := new(struct {
		Since        *big.Int
		CurrentDelay uint32
		PendingDelay uint32
		Effect       *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Since = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.CurrentDelay = *abi.ConvertType(out[1], new(uint32)).(*uint32)
	outstruct.PendingDelay = *abi.ConvertType(out[2], new(uint32)).(*uint32)
	outstruct.Effect = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetAccess is a free data retrieval call binding the contract method 0x3078f114.
//
// Solidity: function getAccess(uint64 roleId, address account) view returns(uint48 since, uint32 currentDelay, uint32 pendingDelay, uint48 effect)
func (_AccessManager *AccessManagerSession) GetAccess(roleId uint64, account common.Address) (struct {
	Since        *big.Int
	CurrentDelay uint32
	PendingDelay uint32
	Effect       *big.Int
}, error) {
	return _AccessManager.Contract.GetAccess(&_AccessManager.CallOpts, roleId, account)
}

// GetAccess is a free data retrieval call binding the contract method 0x3078f114.
//
// Solidity: function getAccess(uint64 roleId, address account) view returns(uint48 since, uint32 currentDelay, uint32 pendingDelay, uint48 effect)
func (_AccessManager *AccessManagerCallerSession) GetAccess(roleId uint64, account common.Address) (struct {
	Since        *big.Int
	CurrentDelay uint32
	PendingDelay uint32
	Effect       *big.Int
}, error) {
	return _AccessManager.Contract.GetAccess(&_AccessManager.CallOpts, roleId, account)
}

// GetNonce is a free data retrieval call binding the contract method 0x4136a33c.
//
// Solidity: function getNonce(bytes32 id) view returns(uint32)
func (_AccessManager *AccessManagerCaller) GetNonce(opts *bind.CallOpts, id [32]byte) (uint32, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getNonce", id)

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// GetNonce is a free data retrieval call binding the contract method 0x4136a33c.
//
// Solidity: function getNonce(bytes32 id) view returns(uint32)
func (_AccessManager *AccessManagerSession) GetNonce(id [32]byte) (uint32, error) {
	return _AccessManager.Contract.GetNonce(&_AccessManager.CallOpts, id)
}

// GetNonce is a free data retrieval call binding the contract method 0x4136a33c.
//
// Solidity: function getNonce(bytes32 id) view returns(uint32)
func (_AccessManager *AccessManagerCallerSession) GetNonce(id [32]byte) (uint32, error) {
	return _AccessManager.Contract.GetNonce(&_AccessManager.CallOpts, id)
}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x530dd456.
//
// Solidity: function getRoleAdmin(uint64 roleId) view returns(uint64)
func (_AccessManager *AccessManagerCaller) GetRoleAdmin(opts *bind.CallOpts, roleId uint64) (uint64, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getRoleAdmin", roleId)

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x530dd456.
//
// Solidity: function getRoleAdmin(uint64 roleId) view returns(uint64)
func (_AccessManager *AccessManagerSession) GetRoleAdmin(roleId uint64) (uint64, error) {
	return _AccessManager.Contract.GetRoleAdmin(&_AccessManager.CallOpts, roleId)
}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x530dd456.
//
// Solidity: function getRoleAdmin(uint64 roleId) view returns(uint64)
func (_AccessManager *AccessManagerCallerSession) GetRoleAdmin(roleId uint64) (uint64, error) {
	return _AccessManager.Contract.GetRoleAdmin(&_AccessManager.CallOpts, roleId)
}

// GetRoleGrantDelay is a free data retrieval call binding the contract method 0x12be8727.
//
// Solidity: function getRoleGrantDelay(uint64 roleId) view returns(uint32)
func (_AccessManager *AccessManagerCaller) GetRoleGrantDelay(opts *bind.CallOpts, roleId uint64) (uint32, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getRoleGrantDelay", roleId)

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// GetRoleGrantDelay is a free data retrieval call binding the contract method 0x12be8727.
//
// Solidity: function getRoleGrantDelay(uint64 roleId) view returns(uint32)
func (_AccessManager *AccessManagerSession) GetRoleGrantDelay(roleId uint64) (uint32, error) {
	return _AccessManager.Contract.GetRoleGrantDelay(&_AccessManager.CallOpts, roleId)
}

// GetRoleGrantDelay is a free data retrieval call binding the contract method 0x12be8727.
//
// Solidity: function getRoleGrantDelay(uint64 roleId) view returns(uint32)
func (_AccessManager *AccessManagerCallerSession) GetRoleGrantDelay(roleId uint64) (uint32, error) {
	return _AccessManager.Contract.GetRoleGrantDelay(&_AccessManager.CallOpts, roleId)
}

// GetRoleGuardian is a free data retrieval call binding the contract method 0x0b0a93ba.
//
// Solidity: function getRoleGuardian(uint64 roleId) view returns(uint64)
func (_AccessManager *AccessManagerCaller) GetRoleGuardian(opts *bind.CallOpts, roleId uint64) (uint64, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getRoleGuardian", roleId)

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetRoleGuardian is a free data retrieval call binding the contract method 0x0b0a93ba.
//
// Solidity: function getRoleGuardian(uint64 roleId) view returns(uint64)
func (_AccessManager *AccessManagerSession) GetRoleGuardian(roleId uint64) (uint64, error) {
	return _AccessManager.Contract.GetRoleGuardian(&_AccessManager.CallOpts, roleId)
}

// GetRoleGuardian is a free data retrieval call binding the contract method 0x0b0a93ba.
//
// Solidity: function getRoleGuardian(uint64 roleId) view returns(uint64)
func (_AccessManager *AccessManagerCallerSession) GetRoleGuardian(roleId uint64) (uint64, error) {
	return _AccessManager.Contract.GetRoleGuardian(&_AccessManager.CallOpts, roleId)
}

// GetSchedule is a free data retrieval call binding the contract method 0x3adc277a.
//
// Solidity: function getSchedule(bytes32 id) view returns(uint48)
func (_AccessManager *AccessManagerCaller) GetSchedule(opts *bind.CallOpts, id [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getSchedule", id)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetSchedule is a free data retrieval call binding the contract method 0x3adc277a.
//
// Solidity: function getSchedule(bytes32 id) view returns(uint48)
func (_AccessManager *AccessManagerSession) GetSchedule(id [32]byte) (*big.Int, error) {
	return _AccessManager.Contract.GetSchedule(&_AccessManager.CallOpts, id)
}

// GetSchedule is a free data retrieval call binding the contract method 0x3adc277a.
//
// Solidity: function getSchedule(bytes32 id) view returns(uint48)
func (_AccessManager *AccessManagerCallerSession) GetSchedule(id [32]byte) (*big.Int, error) {
	return _AccessManager.Contract.GetSchedule(&_AccessManager.CallOpts, id)
}

// GetTargetAdminDelay is a free data retrieval call binding the contract method 0x4c1da1e2.
//
// Solidity: function getTargetAdminDelay(address target) view returns(uint32)
func (_AccessManager *AccessManagerCaller) GetTargetAdminDelay(opts *bind.CallOpts, target common.Address) (uint32, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getTargetAdminDelay", target)

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// GetTargetAdminDelay is a free data retrieval call binding the contract method 0x4c1da1e2.
//
// Solidity: function getTargetAdminDelay(address target) view returns(uint32)
func (_AccessManager *AccessManagerSession) GetTargetAdminDelay(target common.Address) (uint32, error) {
	return _AccessManager.Contract.GetTargetAdminDelay(&_AccessManager.CallOpts, target)
}

// GetTargetAdminDelay is a free data retrieval call binding the contract method 0x4c1da1e2.
//
// Solidity: function getTargetAdminDelay(address target) view returns(uint32)
func (_AccessManager *AccessManagerCallerSession) GetTargetAdminDelay(target common.Address) (uint32, error) {
	return _AccessManager.Contract.GetTargetAdminDelay(&_AccessManager.CallOpts, target)
}

// GetTargetFunctionRole is a free data retrieval call binding the contract method 0x6d5115bd.
//
// Solidity: function getTargetFunctionRole(address target, bytes4 selector) view returns(uint64)
func (_AccessManager *AccessManagerCaller) GetTargetFunctionRole(opts *bind.CallOpts, target common.Address, selector [4]byte) (uint64, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "getTargetFunctionRole", target, selector)

	if err != nil {
		return *new(uint64), err
	}

	out0 := *abi.ConvertType(out[0], new(uint64)).(*uint64)

	return out0, err

}

// GetTargetFunctionRole is a free data retrieval call binding the contract method 0x6d5115bd.
//
// Solidity: function getTargetFunctionRole(address target, bytes4 selector) view returns(uint64)
func (_AccessManager *AccessManagerSession) GetTargetFunctionRole(target common.Address, selector [4]byte) (uint64, error) {
	return _AccessManager.Contract.GetTargetFunctionRole(&_AccessManager.CallOpts, target, selector)
}

// GetTargetFunctionRole is a free data retrieval call binding the contract method 0x6d5115bd.
//
// Solidity: function getTargetFunctionRole(address target, bytes4 selector) view returns(uint64)
func (_AccessManager *AccessManagerCallerSession) GetTargetFunctionRole(target common.Address, selector [4]byte) (uint64, error) {
	return _AccessManager.Contract.GetTargetFunctionRole(&_AccessManager.CallOpts, target, selector)
}

// HasRole is a free data retrieval call binding the contract method 0xd1f856ee.
//
// Solidity: function hasRole(uint64 roleId, address account) view returns(bool isMember, uint32 executionDelay)
func (_AccessManager *AccessManagerCaller) HasRole(opts *bind.CallOpts, roleId uint64, account common.Address) (struct {
	IsMember       bool
	ExecutionDelay uint32
}, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "hasRole", roleId, account)

	outstruct := new(struct {
		IsMember       bool
		ExecutionDelay uint32
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.IsMember = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.ExecutionDelay = *abi.ConvertType(out[1], new(uint32)).(*uint32)

	return *outstruct, err

}

// HasRole is a free data retrieval call binding the contract method 0xd1f856ee.
//
// Solidity: function hasRole(uint64 roleId, address account) view returns(bool isMember, uint32 executionDelay)
func (_AccessManager *AccessManagerSession) HasRole(roleId uint64, account common.Address) (struct {
	IsMember       bool
	ExecutionDelay uint32
}, error) {
	return _AccessManager.Contract.HasRole(&_AccessManager.CallOpts, roleId, account)
}

// HasRole is a free data retrieval call binding the contract method 0xd1f856ee.
//
// Solidity: function hasRole(uint64 roleId, address account) view returns(bool isMember, uint32 executionDelay)
func (_AccessManager *AccessManagerCallerSession) HasRole(roleId uint64, account common.Address) (struct {
	IsMember       bool
	ExecutionDelay uint32
}, error) {
	return _AccessManager.Contract.HasRole(&_AccessManager.CallOpts, roleId, account)
}

// HashOperation is a free data retrieval call binding the contract method 0xabd9bd2a.
//
// Solidity: function hashOperation(address caller, address target, bytes data) view returns(bytes32)
func (_AccessManager *AccessManagerCaller) HashOperation(opts *bind.CallOpts, caller common.Address, target common.Address, data []byte) ([32]byte, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "hashOperation", caller, target, data)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// HashOperation is a free data retrieval call binding the contract method 0xabd9bd2a.
//
// Solidity: function hashOperation(address caller, address target, bytes data) view returns(bytes32)
func (_AccessManager *AccessManagerSession) HashOperation(caller common.Address, target common.Address, data []byte) ([32]byte, error) {
	return _AccessManager.Contract.HashOperation(&_AccessManager.CallOpts, caller, target, data)
}

// HashOperation is a free data retrieval call binding the contract method 0xabd9bd2a.
//
// Solidity: function hashOperation(address caller, address target, bytes data) view returns(bytes32)
func (_AccessManager *AccessManagerCallerSession) HashOperation(caller common.Address, target common.Address, data []byte) ([32]byte, error) {
	return _AccessManager.Contract.HashOperation(&_AccessManager.CallOpts, caller, target, data)
}

// IsTargetClosed is a free data retrieval call binding the contract method 0xa166aa89.
//
// Solidity: function isTargetClosed(address target) view returns(bool)
func (_AccessManager *AccessManagerCaller) IsTargetClosed(opts *bind.CallOpts, target common.Address) (bool, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "isTargetClosed", target)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsTargetClosed is a free data retrieval call binding the contract method 0xa166aa89.
//
// Solidity: function isTargetClosed(address target) view returns(bool)
func (_AccessManager *AccessManagerSession) IsTargetClosed(target common.Address) (bool, error) {
	return _AccessManager.Contract.IsTargetClosed(&_AccessManager.CallOpts, target)
}

// IsTargetClosed is a free data retrieval call binding the contract method 0xa166aa89.
//
// Solidity: function isTargetClosed(address target) view returns(bool)
func (_AccessManager *AccessManagerCallerSession) IsTargetClosed(target common.Address) (bool, error) {
	return _AccessManager.Contract.IsTargetClosed(&_AccessManager.CallOpts, target)
}

// MinSetback is a free data retrieval call binding the contract method 0xcc1b6c81.
//
// Solidity: function minSetback() view returns(uint32)
func (_AccessManager *AccessManagerCaller) MinSetback(opts *bind.CallOpts) (uint32, error) {
	var out []interface{}
	err := _AccessManager.contract.Call(opts, &out, "minSetback")

	if err != nil {
		return *new(uint32), err
	}

	out0 := *abi.ConvertType(out[0], new(uint32)).(*uint32)

	return out0, err

}

// MinSetback is a free data retrieval call binding the contract method 0xcc1b6c81.
//
// Solidity: function minSetback() view returns(uint32)
func (_AccessManager *AccessManagerSession) MinSetback() (uint32, error) {
	return _AccessManager.Contract.MinSetback(&_AccessManager.CallOpts)
}

// MinSetback is a free data retrieval call binding the contract method 0xcc1b6c81.
//
// Solidity: function minSetback() view returns(uint32)
func (_AccessManager *AccessManagerCallerSession) MinSetback() (uint32, error) {
	return _AccessManager.Contract.MinSetback(&_AccessManager.CallOpts)
}

// Cancel is a paid mutator transaction binding the contract method 0xd6bb62c6.
//
// Solidity: function cancel(address caller, address target, bytes data) returns(uint32)
func (_AccessManager *AccessManagerTransactor) Cancel(opts *bind.TransactOpts, caller common.Address, target common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "cancel", caller, target, data)
}

// Cancel is a paid mutator transaction binding the contract method 0xd6bb62c6.
//
// Solidity: function cancel(address caller, address target, bytes data) returns(uint32)
func (_AccessManager *AccessManagerSession) Cancel(caller common.Address, target common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.Contract.Cancel(&_AccessManager.TransactOpts, caller, target, data)
}

// Cancel is a paid mutator transaction binding the contract method 0xd6bb62c6.
//
// Solidity: function cancel(address caller, address target, bytes data) returns(uint32)
func (_AccessManager *AccessManagerTransactorSession) Cancel(caller common.Address, target common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.Contract.Cancel(&_AccessManager.TransactOpts, caller, target, data)
}

// ConsumeScheduledOp is a paid mutator transaction binding the contract method 0x94c7d7ee.
//
// Solidity: function consumeScheduledOp(address caller, bytes data) returns()
func (_AccessManager *AccessManagerTransactor) ConsumeScheduledOp(opts *bind.TransactOpts, caller common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "consumeScheduledOp", caller, data)
}

// ConsumeScheduledOp is a paid mutator transaction binding the contract method 0x94c7d7ee.
//
// Solidity: function consumeScheduledOp(address caller, bytes data) returns()
func (_AccessManager *AccessManagerSession) ConsumeScheduledOp(caller common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.Contract.ConsumeScheduledOp(&_AccessManager.TransactOpts, caller, data)
}

// ConsumeScheduledOp is a paid mutator transaction binding the contract method 0x94c7d7ee.
//
// Solidity: function consumeScheduledOp(address caller, bytes data) returns()
func (_AccessManager *AccessManagerTransactorSession) ConsumeScheduledOp(caller common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.Contract.ConsumeScheduledOp(&_AccessManager.TransactOpts, caller, data)
}

// Execute is a paid mutator transaction binding the contract method 0x1cff79cd.
//
// Solidity: function execute(address target, bytes data) payable returns(uint32)
func (_AccessManager *AccessManagerTransactor) Execute(opts *bind.TransactOpts, target common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "execute", target, data)
}

// Execute is a paid mutator transaction binding the contract method 0x1cff79cd.
//
// Solidity: function execute(address target, bytes data) payable returns(uint32)
func (_AccessManager *AccessManagerSession) Execute(target common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.Contract.Execute(&_AccessManager.TransactOpts, target, data)
}

// Execute is a paid mutator transaction binding the contract method 0x1cff79cd.
//
// Solidity: function execute(address target, bytes data) payable returns(uint32)
func (_AccessManager *AccessManagerTransactorSession) Execute(target common.Address, data []byte) (*types.Transaction, error) {
	return _AccessManager.Contract.Execute(&_AccessManager.TransactOpts, target, data)
}

// GrantRole is a paid mutator transaction binding the contract method 0x25c471a0.
//
// Solidity: function grantRole(uint64 roleId, address account, uint32 executionDelay) returns()
func (_AccessManager *AccessManagerTransactor) GrantRole(opts *bind.TransactOpts, roleId uint64, account common.Address, executionDelay uint32) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "grantRole", roleId, account, executionDelay)
}

// GrantRole is a paid mutator transaction binding the contract method 0x25c471a0.
//
// Solidity: function grantRole(uint64 roleId, address account, uint32 executionDelay) returns()
func (_AccessManager *AccessManagerSession) GrantRole(roleId uint64, account common.Address, executionDelay uint32) (*types.Transaction, error) {
	return _AccessManager.Contract.GrantRole(&_AccessManager.TransactOpts, roleId, account, executionDelay)
}

// GrantRole is a paid mutator transaction binding the contract method 0x25c471a0.
//
// Solidity: function grantRole(uint64 roleId, address account, uint32 executionDelay) returns()
func (_AccessManager *AccessManagerTransactorSession) GrantRole(roleId uint64, account common.Address, executionDelay uint32) (*types.Transaction, error) {
	return _AccessManager.Contract.GrantRole(&_AccessManager.TransactOpts, roleId, account, executionDelay)
}

// LabelRole is a paid mutator transaction binding the contract method 0x853551b8.
//
// Solidity: function labelRole(uint64 roleId, string label) returns()
func (_AccessManager *AccessManagerTransactor) LabelRole(opts *bind.TransactOpts, roleId uint64, label string) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "labelRole", roleId, label)
}

// LabelRole is a paid mutator transaction binding the contract method 0x853551b8.
//
// Solidity: function labelRole(uint64 roleId, string label) returns()
func (_AccessManager *AccessManagerSession) LabelRole(roleId uint64, label string) (*types.Transaction, error) {
	return _AccessManager.Contract.LabelRole(&_AccessManager.TransactOpts, roleId, label)
}

// LabelRole is a paid mutator transaction binding the contract method 0x853551b8.
//
// Solidity: function labelRole(uint64 roleId, string label) returns()
func (_AccessManager *AccessManagerTransactorSession) LabelRole(roleId uint64, label string) (*types.Transaction, error) {
	return _AccessManager.Contract.LabelRole(&_AccessManager.TransactOpts, roleId, label)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns(bytes[] results)
func (_AccessManager *AccessManagerTransactor) Multicall(opts *bind.TransactOpts, data [][]byte) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "multicall", data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns(bytes[] results)
func (_AccessManager *AccessManagerSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _AccessManager.Contract.Multicall(&_AccessManager.TransactOpts, data)
}

// Multicall is a paid mutator transaction binding the contract method 0xac9650d8.
//
// Solidity: function multicall(bytes[] data) returns(bytes[] results)
func (_AccessManager *AccessManagerTransactorSession) Multicall(data [][]byte) (*types.Transaction, error) {
	return _AccessManager.Contract.Multicall(&_AccessManager.TransactOpts, data)
}

// RenounceRole is a paid mutator transaction binding the contract method 0xfe0776f5.
//
// Solidity: function renounceRole(uint64 roleId, address callerConfirmation) returns()
func (_AccessManager *AccessManagerTransactor) RenounceRole(opts *bind.TransactOpts, roleId uint64, callerConfirmation common.Address) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "renounceRole", roleId, callerConfirmation)
}

// RenounceRole is a paid mutator transaction binding the contract method 0xfe0776f5.
//
// Solidity: function renounceRole(uint64 roleId, address callerConfirmation) returns()
func (_AccessManager *AccessManagerSession) RenounceRole(roleId uint64, callerConfirmation common.Address) (*types.Transaction, error) {
	return _AccessManager.Contract.RenounceRole(&_AccessManager.TransactOpts, roleId, callerConfirmation)
}

// RenounceRole is a paid mutator transaction binding the contract method 0xfe0776f5.
//
// Solidity: function renounceRole(uint64 roleId, address callerConfirmation) returns()
func (_AccessManager *AccessManagerTransactorSession) RenounceRole(roleId uint64, callerConfirmation common.Address) (*types.Transaction, error) {
	return _AccessManager.Contract.RenounceRole(&_AccessManager.TransactOpts, roleId, callerConfirmation)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xb7d2b162.
//
// Solidity: function revokeRole(uint64 roleId, address account) returns()
func (_AccessManager *AccessManagerTransactor) RevokeRole(opts *bind.TransactOpts, roleId uint64, account common.Address) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "revokeRole", roleId, account)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xb7d2b162.
//
// Solidity: function revokeRole(uint64 roleId, address account) returns()
func (_AccessManager *AccessManagerSession) RevokeRole(roleId uint64, account common.Address) (*types.Transaction, error) {
	return _AccessManager.Contract.RevokeRole(&_AccessManager.TransactOpts, roleId, account)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xb7d2b162.
//
// Solidity: function revokeRole(uint64 roleId, address account) returns()
func (_AccessManager *AccessManagerTransactorSession) RevokeRole(roleId uint64, account common.Address) (*types.Transaction, error) {
	return _AccessManager.Contract.RevokeRole(&_AccessManager.TransactOpts, roleId, account)
}

// Schedule is a paid mutator transaction binding the contract method 0xf801a698.
//
// Solidity: function schedule(address target, bytes data, uint48 when) returns(bytes32 operationId, uint32 nonce)
func (_AccessManager *AccessManagerTransactor) Schedule(opts *bind.TransactOpts, target common.Address, data []byte, when *big.Int) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "schedule", target, data, when)
}

// Schedule is a paid mutator transaction binding the contract method 0xf801a698.
//
// Solidity: function schedule(address target, bytes data, uint48 when) returns(bytes32 operationId, uint32 nonce)
func (_AccessManager *AccessManagerSession) Schedule(target common.Address, data []byte, when *big.Int) (*types.Transaction, error) {
	return _AccessManager.Contract.Schedule(&_AccessManager.TransactOpts, target, data, when)
}

// Schedule is a paid mutator transaction binding the contract method 0xf801a698.
//
// Solidity: function schedule(address target, bytes data, uint48 when) returns(bytes32 operationId, uint32 nonce)
func (_AccessManager *AccessManagerTransactorSession) Schedule(target common.Address, data []byte, when *big.Int) (*types.Transaction, error) {
	return _AccessManager.Contract.Schedule(&_AccessManager.TransactOpts, target, data, when)
}

// SetGrantDelay is a paid mutator transaction binding the contract method 0xa64d95ce.
//
// Solidity: function setGrantDelay(uint64 roleId, uint32 newDelay) returns()
func (_AccessManager *AccessManagerTransactor) SetGrantDelay(opts *bind.TransactOpts, roleId uint64, newDelay uint32) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "setGrantDelay", roleId, newDelay)
}

// SetGrantDelay is a paid mutator transaction binding the contract method 0xa64d95ce.
//
// Solidity: function setGrantDelay(uint64 roleId, uint32 newDelay) returns()
func (_AccessManager *AccessManagerSession) SetGrantDelay(roleId uint64, newDelay uint32) (*types.Transaction, error) {
	return _AccessManager.Contract.SetGrantDelay(&_AccessManager.TransactOpts, roleId, newDelay)
}

// SetGrantDelay is a paid mutator transaction binding the contract method 0xa64d95ce.
//
// Solidity: function setGrantDelay(uint64 roleId, uint32 newDelay) returns()
func (_AccessManager *AccessManagerTransactorSession) SetGrantDelay(roleId uint64, newDelay uint32) (*types.Transaction, error) {
	return _AccessManager.Contract.SetGrantDelay(&_AccessManager.TransactOpts, roleId, newDelay)
}

// SetRoleAdmin is a paid mutator transaction binding the contract method 0x30cae187.
//
// Solidity: function setRoleAdmin(uint64 roleId, uint64 admin) returns()
func (_AccessManager *AccessManagerTransactor) SetRoleAdmin(opts *bind.TransactOpts, roleId uint64, admin uint64) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "setRoleAdmin", roleId, admin)
}

// SetRoleAdmin is a paid mutator transaction binding the contract method 0x30cae187.
//
// Solidity: function setRoleAdmin(uint64 roleId, uint64 admin) returns()
func (_AccessManager *AccessManagerSession) SetRoleAdmin(roleId uint64, admin uint64) (*types.Transaction, error) {
	return _AccessManager.Contract.SetRoleAdmin(&_AccessManager.TransactOpts, roleId, admin)
}

// SetRoleAdmin is a paid mutator transaction binding the contract method 0x30cae187.
//
// Solidity: function setRoleAdmin(uint64 roleId, uint64 admin) returns()
func (_AccessManager *AccessManagerTransactorSession) SetRoleAdmin(roleId uint64, admin uint64) (*types.Transaction, error) {
	return _AccessManager.Contract.SetRoleAdmin(&_AccessManager.TransactOpts, roleId, admin)
}

// SetRoleGuardian is a paid mutator transaction binding the contract method 0x52962952.
//
// Solidity: function setRoleGuardian(uint64 roleId, uint64 guardian) returns()
func (_AccessManager *AccessManagerTransactor) SetRoleGuardian(opts *bind.TransactOpts, roleId uint64, guardian uint64) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "setRoleGuardian", roleId, guardian)
}

// SetRoleGuardian is a paid mutator transaction binding the contract method 0x52962952.
//
// Solidity: function setRoleGuardian(uint64 roleId, uint64 guardian) returns()
func (_AccessManager *AccessManagerSession) SetRoleGuardian(roleId uint64, guardian uint64) (*types.Transaction, error) {
	return _AccessManager.Contract.SetRoleGuardian(&_AccessManager.TransactOpts, roleId, guardian)
}

// SetRoleGuardian is a paid mutator transaction binding the contract method 0x52962952.
//
// Solidity: function setRoleGuardian(uint64 roleId, uint64 guardian) returns()
func (_AccessManager *AccessManagerTransactorSession) SetRoleGuardian(roleId uint64, guardian uint64) (*types.Transaction, error) {
	return _AccessManager.Contract.SetRoleGuardian(&_AccessManager.TransactOpts, roleId, guardian)
}

// SetTargetAdminDelay is a paid mutator transaction binding the contract method 0xd22b5989.
//
// Solidity: function setTargetAdminDelay(address target, uint32 newDelay) returns()
func (_AccessManager *AccessManagerTransactor) SetTargetAdminDelay(opts *bind.TransactOpts, target common.Address, newDelay uint32) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "setTargetAdminDelay", target, newDelay)
}

// SetTargetAdminDelay is a paid mutator transaction binding the contract method 0xd22b5989.
//
// Solidity: function setTargetAdminDelay(address target, uint32 newDelay) returns()
func (_AccessManager *AccessManagerSession) SetTargetAdminDelay(target common.Address, newDelay uint32) (*types.Transaction, error) {
	return _AccessManager.Contract.SetTargetAdminDelay(&_AccessManager.TransactOpts, target, newDelay)
}

// SetTargetAdminDelay is a paid mutator transaction binding the contract method 0xd22b5989.
//
// Solidity: function setTargetAdminDelay(address target, uint32 newDelay) returns()
func (_AccessManager *AccessManagerTransactorSession) SetTargetAdminDelay(target common.Address, newDelay uint32) (*types.Transaction, error) {
	return _AccessManager.Contract.SetTargetAdminDelay(&_AccessManager.TransactOpts, target, newDelay)
}

// SetTargetClosed is a paid mutator transaction binding the contract method 0x167bd395.
//
// Solidity: function setTargetClosed(address target, bool closed) returns()
func (_AccessManager *AccessManagerTransactor) SetTargetClosed(opts *bind.TransactOpts, target common.Address, closed bool) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "setTargetClosed", target, closed)
}

// SetTargetClosed is a paid mutator transaction binding the contract method 0x167bd395.
//
// Solidity: function setTargetClosed(address target, bool closed) returns()
func (_AccessManager *AccessManagerSession) SetTargetClosed(target common.Address, closed bool) (*types.Transaction, error) {
	return _AccessManager.Contract.SetTargetClosed(&_AccessManager.TransactOpts, target, closed)
}

// SetTargetClosed is a paid mutator transaction binding the contract method 0x167bd395.
//
// Solidity: function setTargetClosed(address target, bool closed) returns()
func (_AccessManager *AccessManagerTransactorSession) SetTargetClosed(target common.Address, closed bool) (*types.Transaction, error) {
	return _AccessManager.Contract.SetTargetClosed(&_AccessManager.TransactOpts, target, closed)
}

// SetTargetFunctionRole is a paid mutator transaction binding the contract method 0x08d6122d.
//
// Solidity: function setTargetFunctionRole(address target, bytes4[] selectors, uint64 roleId) returns()
func (_AccessManager *AccessManagerTransactor) SetTargetFunctionRole(opts *bind.TransactOpts, target common.Address, selectors [][4]byte, roleId uint64) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "setTargetFunctionRole", target, selectors, roleId)
}

// SetTargetFunctionRole is a paid mutator transaction binding the contract method 0x08d6122d.
//
// Solidity: function setTargetFunctionRole(address target, bytes4[] selectors, uint64 roleId) returns()
func (_AccessManager *AccessManagerSession) SetTargetFunctionRole(target common.Address, selectors [][4]byte, roleId uint64) (*types.Transaction, error) {
	return _AccessManager.Contract.SetTargetFunctionRole(&_AccessManager.TransactOpts, target, selectors, roleId)
}

// SetTargetFunctionRole is a paid mutator transaction binding the contract method 0x08d6122d.
//
// Solidity: function setTargetFunctionRole(address target, bytes4[] selectors, uint64 roleId) returns()
func (_AccessManager *AccessManagerTransactorSession) SetTargetFunctionRole(target common.Address, selectors [][4]byte, roleId uint64) (*types.Transaction, error) {
	return _AccessManager.Contract.SetTargetFunctionRole(&_AccessManager.TransactOpts, target, selectors, roleId)
}

// UpdateAuthority is a paid mutator transaction binding the contract method 0x18ff183c.
//
// Solidity: function updateAuthority(address target, address newAuthority) returns()
func (_AccessManager *AccessManagerTransactor) UpdateAuthority(opts *bind.TransactOpts, target common.Address, newAuthority common.Address) (*types.Transaction, error) {
	return _AccessManager.contract.Transact(opts, "updateAuthority", target, newAuthority)
}

// UpdateAuthority is a paid mutator transaction binding the contract method 0x18ff183c.
//
// Solidity: function updateAuthority(address target, address newAuthority) returns()
func (_AccessManager *AccessManagerSession) UpdateAuthority(target common.Address, newAuthority common.Address) (*types.Transaction, error) {
	return _AccessManager.Contract.UpdateAuthority(&_AccessManager.TransactOpts, target, newAuthority)
}

// UpdateAuthority is a paid mutator transaction binding the contract method 0x18ff183c.
//
// Solidity: function updateAuthority(address target, address newAuthority) returns()
func (_AccessManager *AccessManagerTransactorSession) UpdateAuthority(target common.Address, newAuthority common.Address) (*types.Transaction, error) {
	return _AccessManager.Contract.UpdateAuthority(&_AccessManager.TransactOpts, target, newAuthority)
}

// AccessManagerOperationCanceledIterator is returned from FilterOperationCanceled and is used to iterate over the raw logs and unpacked data for OperationCanceled events raised by the AccessManager contract.
type AccessManagerOperationCanceledIterator struct {
	Event *AccessManagerOperationCanceled // Event containing the contract specifics and raw log

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
func (it *AccessManagerOperationCanceledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerOperationCanceled)
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
		it.Event = new(AccessManagerOperationCanceled)
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
func (it *AccessManagerOperationCanceledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerOperationCanceledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerOperationCanceled represents a OperationCanceled event raised by the AccessManager contract.
type AccessManagerOperationCanceled struct {
	OperationId [32]byte
	Nonce       uint32
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOperationCanceled is a free log retrieval operation binding the contract event 0xbd9ac67a6e2f6463b80927326310338bcbb4bdb7936ce1365ea3e01067e7b9f7.
//
// Solidity: event OperationCanceled(bytes32 indexed operationId, uint32 indexed nonce)
func (_AccessManager *AccessManagerFilterer) FilterOperationCanceled(opts *bind.FilterOpts, operationId [][32]byte, nonce []uint32) (*AccessManagerOperationCanceledIterator, error) {

	var operationIdRule []interface{}
	for _, operationIdItem := range operationId {
		operationIdRule = append(operationIdRule, operationIdItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "OperationCanceled", operationIdRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerOperationCanceledIterator{contract: _AccessManager.contract, event: "OperationCanceled", logs: logs, sub: sub}, nil
}

// WatchOperationCanceled is a free log subscription operation binding the contract event 0xbd9ac67a6e2f6463b80927326310338bcbb4bdb7936ce1365ea3e01067e7b9f7.
//
// Solidity: event OperationCanceled(bytes32 indexed operationId, uint32 indexed nonce)
func (_AccessManager *AccessManagerFilterer) WatchOperationCanceled(opts *bind.WatchOpts, sink chan<- *AccessManagerOperationCanceled, operationId [][32]byte, nonce []uint32) (event.Subscription, error) {

	var operationIdRule []interface{}
	for _, operationIdItem := range operationId {
		operationIdRule = append(operationIdRule, operationIdItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "OperationCanceled", operationIdRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerOperationCanceled)
				if err := _AccessManager.contract.UnpackLog(event, "OperationCanceled", log); err != nil {
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

// ParseOperationCanceled is a log parse operation binding the contract event 0xbd9ac67a6e2f6463b80927326310338bcbb4bdb7936ce1365ea3e01067e7b9f7.
//
// Solidity: event OperationCanceled(bytes32 indexed operationId, uint32 indexed nonce)
func (_AccessManager *AccessManagerFilterer) ParseOperationCanceled(log types.Log) (*AccessManagerOperationCanceled, error) {
	event := new(AccessManagerOperationCanceled)
	if err := _AccessManager.contract.UnpackLog(event, "OperationCanceled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerOperationExecutedIterator is returned from FilterOperationExecuted and is used to iterate over the raw logs and unpacked data for OperationExecuted events raised by the AccessManager contract.
type AccessManagerOperationExecutedIterator struct {
	Event *AccessManagerOperationExecuted // Event containing the contract specifics and raw log

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
func (it *AccessManagerOperationExecutedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerOperationExecuted)
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
		it.Event = new(AccessManagerOperationExecuted)
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
func (it *AccessManagerOperationExecutedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerOperationExecutedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerOperationExecuted represents a OperationExecuted event raised by the AccessManager contract.
type AccessManagerOperationExecuted struct {
	OperationId [32]byte
	Nonce       uint32
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOperationExecuted is a free log retrieval operation binding the contract event 0x76a2a46953689d4861a5d3f6ed883ad7e6af674a21f8e162707159fc9dde614d.
//
// Solidity: event OperationExecuted(bytes32 indexed operationId, uint32 indexed nonce)
func (_AccessManager *AccessManagerFilterer) FilterOperationExecuted(opts *bind.FilterOpts, operationId [][32]byte, nonce []uint32) (*AccessManagerOperationExecutedIterator, error) {

	var operationIdRule []interface{}
	for _, operationIdItem := range operationId {
		operationIdRule = append(operationIdRule, operationIdItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "OperationExecuted", operationIdRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerOperationExecutedIterator{contract: _AccessManager.contract, event: "OperationExecuted", logs: logs, sub: sub}, nil
}

// WatchOperationExecuted is a free log subscription operation binding the contract event 0x76a2a46953689d4861a5d3f6ed883ad7e6af674a21f8e162707159fc9dde614d.
//
// Solidity: event OperationExecuted(bytes32 indexed operationId, uint32 indexed nonce)
func (_AccessManager *AccessManagerFilterer) WatchOperationExecuted(opts *bind.WatchOpts, sink chan<- *AccessManagerOperationExecuted, operationId [][32]byte, nonce []uint32) (event.Subscription, error) {

	var operationIdRule []interface{}
	for _, operationIdItem := range operationId {
		operationIdRule = append(operationIdRule, operationIdItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "OperationExecuted", operationIdRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerOperationExecuted)
				if err := _AccessManager.contract.UnpackLog(event, "OperationExecuted", log); err != nil {
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

// ParseOperationExecuted is a log parse operation binding the contract event 0x76a2a46953689d4861a5d3f6ed883ad7e6af674a21f8e162707159fc9dde614d.
//
// Solidity: event OperationExecuted(bytes32 indexed operationId, uint32 indexed nonce)
func (_AccessManager *AccessManagerFilterer) ParseOperationExecuted(log types.Log) (*AccessManagerOperationExecuted, error) {
	event := new(AccessManagerOperationExecuted)
	if err := _AccessManager.contract.UnpackLog(event, "OperationExecuted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerOperationScheduledIterator is returned from FilterOperationScheduled and is used to iterate over the raw logs and unpacked data for OperationScheduled events raised by the AccessManager contract.
type AccessManagerOperationScheduledIterator struct {
	Event *AccessManagerOperationScheduled // Event containing the contract specifics and raw log

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
func (it *AccessManagerOperationScheduledIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerOperationScheduled)
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
		it.Event = new(AccessManagerOperationScheduled)
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
func (it *AccessManagerOperationScheduledIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerOperationScheduledIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerOperationScheduled represents a OperationScheduled event raised by the AccessManager contract.
type AccessManagerOperationScheduled struct {
	OperationId [32]byte
	Nonce       uint32
	Schedule    *big.Int
	Caller      common.Address
	Target      common.Address
	Data        []byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterOperationScheduled is a free log retrieval operation binding the contract event 0x82a2da5dee54ea8021c6545b4444620291e07ee83be6dd57edb175062715f3b4.
//
// Solidity: event OperationScheduled(bytes32 indexed operationId, uint32 indexed nonce, uint48 schedule, address caller, address target, bytes data)
func (_AccessManager *AccessManagerFilterer) FilterOperationScheduled(opts *bind.FilterOpts, operationId [][32]byte, nonce []uint32) (*AccessManagerOperationScheduledIterator, error) {

	var operationIdRule []interface{}
	for _, operationIdItem := range operationId {
		operationIdRule = append(operationIdRule, operationIdItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "OperationScheduled", operationIdRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerOperationScheduledIterator{contract: _AccessManager.contract, event: "OperationScheduled", logs: logs, sub: sub}, nil
}

// WatchOperationScheduled is a free log subscription operation binding the contract event 0x82a2da5dee54ea8021c6545b4444620291e07ee83be6dd57edb175062715f3b4.
//
// Solidity: event OperationScheduled(bytes32 indexed operationId, uint32 indexed nonce, uint48 schedule, address caller, address target, bytes data)
func (_AccessManager *AccessManagerFilterer) WatchOperationScheduled(opts *bind.WatchOpts, sink chan<- *AccessManagerOperationScheduled, operationId [][32]byte, nonce []uint32) (event.Subscription, error) {

	var operationIdRule []interface{}
	for _, operationIdItem := range operationId {
		operationIdRule = append(operationIdRule, operationIdItem)
	}
	var nonceRule []interface{}
	for _, nonceItem := range nonce {
		nonceRule = append(nonceRule, nonceItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "OperationScheduled", operationIdRule, nonceRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerOperationScheduled)
				if err := _AccessManager.contract.UnpackLog(event, "OperationScheduled", log); err != nil {
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

// ParseOperationScheduled is a log parse operation binding the contract event 0x82a2da5dee54ea8021c6545b4444620291e07ee83be6dd57edb175062715f3b4.
//
// Solidity: event OperationScheduled(bytes32 indexed operationId, uint32 indexed nonce, uint48 schedule, address caller, address target, bytes data)
func (_AccessManager *AccessManagerFilterer) ParseOperationScheduled(log types.Log) (*AccessManagerOperationScheduled, error) {
	event := new(AccessManagerOperationScheduled)
	if err := _AccessManager.contract.UnpackLog(event, "OperationScheduled", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerRoleAdminChangedIterator is returned from FilterRoleAdminChanged and is used to iterate over the raw logs and unpacked data for RoleAdminChanged events raised by the AccessManager contract.
type AccessManagerRoleAdminChangedIterator struct {
	Event *AccessManagerRoleAdminChanged // Event containing the contract specifics and raw log

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
func (it *AccessManagerRoleAdminChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerRoleAdminChanged)
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
		it.Event = new(AccessManagerRoleAdminChanged)
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
func (it *AccessManagerRoleAdminChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerRoleAdminChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerRoleAdminChanged represents a RoleAdminChanged event raised by the AccessManager contract.
type AccessManagerRoleAdminChanged struct {
	RoleId uint64
	Admin  uint64
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRoleAdminChanged is a free log retrieval operation binding the contract event 0x1fd6dd7631312dfac2205b52913f99de03b4d7e381d5d27d3dbfe0713e6e6340.
//
// Solidity: event RoleAdminChanged(uint64 indexed roleId, uint64 indexed admin)
func (_AccessManager *AccessManagerFilterer) FilterRoleAdminChanged(opts *bind.FilterOpts, roleId []uint64, admin []uint64) (*AccessManagerRoleAdminChangedIterator, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var adminRule []interface{}
	for _, adminItem := range admin {
		adminRule = append(adminRule, adminItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "RoleAdminChanged", roleIdRule, adminRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerRoleAdminChangedIterator{contract: _AccessManager.contract, event: "RoleAdminChanged", logs: logs, sub: sub}, nil
}

// WatchRoleAdminChanged is a free log subscription operation binding the contract event 0x1fd6dd7631312dfac2205b52913f99de03b4d7e381d5d27d3dbfe0713e6e6340.
//
// Solidity: event RoleAdminChanged(uint64 indexed roleId, uint64 indexed admin)
func (_AccessManager *AccessManagerFilterer) WatchRoleAdminChanged(opts *bind.WatchOpts, sink chan<- *AccessManagerRoleAdminChanged, roleId []uint64, admin []uint64) (event.Subscription, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var adminRule []interface{}
	for _, adminItem := range admin {
		adminRule = append(adminRule, adminItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "RoleAdminChanged", roleIdRule, adminRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerRoleAdminChanged)
				if err := _AccessManager.contract.UnpackLog(event, "RoleAdminChanged", log); err != nil {
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

// ParseRoleAdminChanged is a log parse operation binding the contract event 0x1fd6dd7631312dfac2205b52913f99de03b4d7e381d5d27d3dbfe0713e6e6340.
//
// Solidity: event RoleAdminChanged(uint64 indexed roleId, uint64 indexed admin)
func (_AccessManager *AccessManagerFilterer) ParseRoleAdminChanged(log types.Log) (*AccessManagerRoleAdminChanged, error) {
	event := new(AccessManagerRoleAdminChanged)
	if err := _AccessManager.contract.UnpackLog(event, "RoleAdminChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerRoleGrantDelayChangedIterator is returned from FilterRoleGrantDelayChanged and is used to iterate over the raw logs and unpacked data for RoleGrantDelayChanged events raised by the AccessManager contract.
type AccessManagerRoleGrantDelayChangedIterator struct {
	Event *AccessManagerRoleGrantDelayChanged // Event containing the contract specifics and raw log

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
func (it *AccessManagerRoleGrantDelayChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerRoleGrantDelayChanged)
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
		it.Event = new(AccessManagerRoleGrantDelayChanged)
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
func (it *AccessManagerRoleGrantDelayChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerRoleGrantDelayChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerRoleGrantDelayChanged represents a RoleGrantDelayChanged event raised by the AccessManager contract.
type AccessManagerRoleGrantDelayChanged struct {
	RoleId uint64
	Delay  uint32
	Since  *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRoleGrantDelayChanged is a free log retrieval operation binding the contract event 0xfeb69018ee8b8fd50ea86348f1267d07673379f72cffdeccec63853ee8ce8b48.
//
// Solidity: event RoleGrantDelayChanged(uint64 indexed roleId, uint32 delay, uint48 since)
func (_AccessManager *AccessManagerFilterer) FilterRoleGrantDelayChanged(opts *bind.FilterOpts, roleId []uint64) (*AccessManagerRoleGrantDelayChangedIterator, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "RoleGrantDelayChanged", roleIdRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerRoleGrantDelayChangedIterator{contract: _AccessManager.contract, event: "RoleGrantDelayChanged", logs: logs, sub: sub}, nil
}

// WatchRoleGrantDelayChanged is a free log subscription operation binding the contract event 0xfeb69018ee8b8fd50ea86348f1267d07673379f72cffdeccec63853ee8ce8b48.
//
// Solidity: event RoleGrantDelayChanged(uint64 indexed roleId, uint32 delay, uint48 since)
func (_AccessManager *AccessManagerFilterer) WatchRoleGrantDelayChanged(opts *bind.WatchOpts, sink chan<- *AccessManagerRoleGrantDelayChanged, roleId []uint64) (event.Subscription, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "RoleGrantDelayChanged", roleIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerRoleGrantDelayChanged)
				if err := _AccessManager.contract.UnpackLog(event, "RoleGrantDelayChanged", log); err != nil {
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

// ParseRoleGrantDelayChanged is a log parse operation binding the contract event 0xfeb69018ee8b8fd50ea86348f1267d07673379f72cffdeccec63853ee8ce8b48.
//
// Solidity: event RoleGrantDelayChanged(uint64 indexed roleId, uint32 delay, uint48 since)
func (_AccessManager *AccessManagerFilterer) ParseRoleGrantDelayChanged(log types.Log) (*AccessManagerRoleGrantDelayChanged, error) {
	event := new(AccessManagerRoleGrantDelayChanged)
	if err := _AccessManager.contract.UnpackLog(event, "RoleGrantDelayChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerRoleGrantedIterator is returned from FilterRoleGranted and is used to iterate over the raw logs and unpacked data for RoleGranted events raised by the AccessManager contract.
type AccessManagerRoleGrantedIterator struct {
	Event *AccessManagerRoleGranted // Event containing the contract specifics and raw log

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
func (it *AccessManagerRoleGrantedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerRoleGranted)
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
		it.Event = new(AccessManagerRoleGranted)
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
func (it *AccessManagerRoleGrantedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerRoleGrantedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerRoleGranted represents a RoleGranted event raised by the AccessManager contract.
type AccessManagerRoleGranted struct {
	RoleId    uint64
	Account   common.Address
	Delay     uint32
	Since     *big.Int
	NewMember bool
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterRoleGranted is a free log retrieval operation binding the contract event 0xf98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf.
//
// Solidity: event RoleGranted(uint64 indexed roleId, address indexed account, uint32 delay, uint48 since, bool newMember)
func (_AccessManager *AccessManagerFilterer) FilterRoleGranted(opts *bind.FilterOpts, roleId []uint64, account []common.Address) (*AccessManagerRoleGrantedIterator, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "RoleGranted", roleIdRule, accountRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerRoleGrantedIterator{contract: _AccessManager.contract, event: "RoleGranted", logs: logs, sub: sub}, nil
}

// WatchRoleGranted is a free log subscription operation binding the contract event 0xf98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf.
//
// Solidity: event RoleGranted(uint64 indexed roleId, address indexed account, uint32 delay, uint48 since, bool newMember)
func (_AccessManager *AccessManagerFilterer) WatchRoleGranted(opts *bind.WatchOpts, sink chan<- *AccessManagerRoleGranted, roleId []uint64, account []common.Address) (event.Subscription, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "RoleGranted", roleIdRule, accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerRoleGranted)
				if err := _AccessManager.contract.UnpackLog(event, "RoleGranted", log); err != nil {
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

// ParseRoleGranted is a log parse operation binding the contract event 0xf98448b987f1428e0e230e1f3c6e2ce15b5693eaf31827fbd0b1ec4b424ae7cf.
//
// Solidity: event RoleGranted(uint64 indexed roleId, address indexed account, uint32 delay, uint48 since, bool newMember)
func (_AccessManager *AccessManagerFilterer) ParseRoleGranted(log types.Log) (*AccessManagerRoleGranted, error) {
	event := new(AccessManagerRoleGranted)
	if err := _AccessManager.contract.UnpackLog(event, "RoleGranted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerRoleGuardianChangedIterator is returned from FilterRoleGuardianChanged and is used to iterate over the raw logs and unpacked data for RoleGuardianChanged events raised by the AccessManager contract.
type AccessManagerRoleGuardianChangedIterator struct {
	Event *AccessManagerRoleGuardianChanged // Event containing the contract specifics and raw log

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
func (it *AccessManagerRoleGuardianChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerRoleGuardianChanged)
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
		it.Event = new(AccessManagerRoleGuardianChanged)
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
func (it *AccessManagerRoleGuardianChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerRoleGuardianChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerRoleGuardianChanged represents a RoleGuardianChanged event raised by the AccessManager contract.
type AccessManagerRoleGuardianChanged struct {
	RoleId   uint64
	Guardian uint64
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterRoleGuardianChanged is a free log retrieval operation binding the contract event 0x7a8059630b897b5de4c08ade69f8b90c3ead1f8596d62d10b6c4d14a0afb4ae2.
//
// Solidity: event RoleGuardianChanged(uint64 indexed roleId, uint64 indexed guardian)
func (_AccessManager *AccessManagerFilterer) FilterRoleGuardianChanged(opts *bind.FilterOpts, roleId []uint64, guardian []uint64) (*AccessManagerRoleGuardianChangedIterator, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var guardianRule []interface{}
	for _, guardianItem := range guardian {
		guardianRule = append(guardianRule, guardianItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "RoleGuardianChanged", roleIdRule, guardianRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerRoleGuardianChangedIterator{contract: _AccessManager.contract, event: "RoleGuardianChanged", logs: logs, sub: sub}, nil
}

// WatchRoleGuardianChanged is a free log subscription operation binding the contract event 0x7a8059630b897b5de4c08ade69f8b90c3ead1f8596d62d10b6c4d14a0afb4ae2.
//
// Solidity: event RoleGuardianChanged(uint64 indexed roleId, uint64 indexed guardian)
func (_AccessManager *AccessManagerFilterer) WatchRoleGuardianChanged(opts *bind.WatchOpts, sink chan<- *AccessManagerRoleGuardianChanged, roleId []uint64, guardian []uint64) (event.Subscription, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var guardianRule []interface{}
	for _, guardianItem := range guardian {
		guardianRule = append(guardianRule, guardianItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "RoleGuardianChanged", roleIdRule, guardianRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerRoleGuardianChanged)
				if err := _AccessManager.contract.UnpackLog(event, "RoleGuardianChanged", log); err != nil {
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

// ParseRoleGuardianChanged is a log parse operation binding the contract event 0x7a8059630b897b5de4c08ade69f8b90c3ead1f8596d62d10b6c4d14a0afb4ae2.
//
// Solidity: event RoleGuardianChanged(uint64 indexed roleId, uint64 indexed guardian)
func (_AccessManager *AccessManagerFilterer) ParseRoleGuardianChanged(log types.Log) (*AccessManagerRoleGuardianChanged, error) {
	event := new(AccessManagerRoleGuardianChanged)
	if err := _AccessManager.contract.UnpackLog(event, "RoleGuardianChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerRoleLabelIterator is returned from FilterRoleLabel and is used to iterate over the raw logs and unpacked data for RoleLabel events raised by the AccessManager contract.
type AccessManagerRoleLabelIterator struct {
	Event *AccessManagerRoleLabel // Event containing the contract specifics and raw log

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
func (it *AccessManagerRoleLabelIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerRoleLabel)
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
		it.Event = new(AccessManagerRoleLabel)
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
func (it *AccessManagerRoleLabelIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerRoleLabelIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerRoleLabel represents a RoleLabel event raised by the AccessManager contract.
type AccessManagerRoleLabel struct {
	RoleId uint64
	Label  string
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterRoleLabel is a free log retrieval operation binding the contract event 0x1256f5b5ecb89caec12db449738f2fbcd1ba5806cf38f35413f4e5c15bf6a450.
//
// Solidity: event RoleLabel(uint64 indexed roleId, string label)
func (_AccessManager *AccessManagerFilterer) FilterRoleLabel(opts *bind.FilterOpts, roleId []uint64) (*AccessManagerRoleLabelIterator, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "RoleLabel", roleIdRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerRoleLabelIterator{contract: _AccessManager.contract, event: "RoleLabel", logs: logs, sub: sub}, nil
}

// WatchRoleLabel is a free log subscription operation binding the contract event 0x1256f5b5ecb89caec12db449738f2fbcd1ba5806cf38f35413f4e5c15bf6a450.
//
// Solidity: event RoleLabel(uint64 indexed roleId, string label)
func (_AccessManager *AccessManagerFilterer) WatchRoleLabel(opts *bind.WatchOpts, sink chan<- *AccessManagerRoleLabel, roleId []uint64) (event.Subscription, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "RoleLabel", roleIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerRoleLabel)
				if err := _AccessManager.contract.UnpackLog(event, "RoleLabel", log); err != nil {
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

// ParseRoleLabel is a log parse operation binding the contract event 0x1256f5b5ecb89caec12db449738f2fbcd1ba5806cf38f35413f4e5c15bf6a450.
//
// Solidity: event RoleLabel(uint64 indexed roleId, string label)
func (_AccessManager *AccessManagerFilterer) ParseRoleLabel(log types.Log) (*AccessManagerRoleLabel, error) {
	event := new(AccessManagerRoleLabel)
	if err := _AccessManager.contract.UnpackLog(event, "RoleLabel", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerRoleRevokedIterator is returned from FilterRoleRevoked and is used to iterate over the raw logs and unpacked data for RoleRevoked events raised by the AccessManager contract.
type AccessManagerRoleRevokedIterator struct {
	Event *AccessManagerRoleRevoked // Event containing the contract specifics and raw log

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
func (it *AccessManagerRoleRevokedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerRoleRevoked)
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
		it.Event = new(AccessManagerRoleRevoked)
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
func (it *AccessManagerRoleRevokedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerRoleRevokedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerRoleRevoked represents a RoleRevoked event raised by the AccessManager contract.
type AccessManagerRoleRevoked struct {
	RoleId  uint64
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRoleRevoked is a free log retrieval operation binding the contract event 0xf229baa593af28c41b1d16b748cd7688f0c83aaf92d4be41c44005defe84c166.
//
// Solidity: event RoleRevoked(uint64 indexed roleId, address indexed account)
func (_AccessManager *AccessManagerFilterer) FilterRoleRevoked(opts *bind.FilterOpts, roleId []uint64, account []common.Address) (*AccessManagerRoleRevokedIterator, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "RoleRevoked", roleIdRule, accountRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerRoleRevokedIterator{contract: _AccessManager.contract, event: "RoleRevoked", logs: logs, sub: sub}, nil
}

// WatchRoleRevoked is a free log subscription operation binding the contract event 0xf229baa593af28c41b1d16b748cd7688f0c83aaf92d4be41c44005defe84c166.
//
// Solidity: event RoleRevoked(uint64 indexed roleId, address indexed account)
func (_AccessManager *AccessManagerFilterer) WatchRoleRevoked(opts *bind.WatchOpts, sink chan<- *AccessManagerRoleRevoked, roleId []uint64, account []common.Address) (event.Subscription, error) {

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "RoleRevoked", roleIdRule, accountRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerRoleRevoked)
				if err := _AccessManager.contract.UnpackLog(event, "RoleRevoked", log); err != nil {
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

// ParseRoleRevoked is a log parse operation binding the contract event 0xf229baa593af28c41b1d16b748cd7688f0c83aaf92d4be41c44005defe84c166.
//
// Solidity: event RoleRevoked(uint64 indexed roleId, address indexed account)
func (_AccessManager *AccessManagerFilterer) ParseRoleRevoked(log types.Log) (*AccessManagerRoleRevoked, error) {
	event := new(AccessManagerRoleRevoked)
	if err := _AccessManager.contract.UnpackLog(event, "RoleRevoked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerTargetAdminDelayUpdatedIterator is returned from FilterTargetAdminDelayUpdated and is used to iterate over the raw logs and unpacked data for TargetAdminDelayUpdated events raised by the AccessManager contract.
type AccessManagerTargetAdminDelayUpdatedIterator struct {
	Event *AccessManagerTargetAdminDelayUpdated // Event containing the contract specifics and raw log

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
func (it *AccessManagerTargetAdminDelayUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerTargetAdminDelayUpdated)
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
		it.Event = new(AccessManagerTargetAdminDelayUpdated)
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
func (it *AccessManagerTargetAdminDelayUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerTargetAdminDelayUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerTargetAdminDelayUpdated represents a TargetAdminDelayUpdated event raised by the AccessManager contract.
type AccessManagerTargetAdminDelayUpdated struct {
	Target common.Address
	Delay  uint32
	Since  *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTargetAdminDelayUpdated is a free log retrieval operation binding the contract event 0xa56b76017453f399ec2327ba00375dbfb1fd070ff854341ad6191e6a2e2de19c.
//
// Solidity: event TargetAdminDelayUpdated(address indexed target, uint32 delay, uint48 since)
func (_AccessManager *AccessManagerFilterer) FilterTargetAdminDelayUpdated(opts *bind.FilterOpts, target []common.Address) (*AccessManagerTargetAdminDelayUpdatedIterator, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "TargetAdminDelayUpdated", targetRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerTargetAdminDelayUpdatedIterator{contract: _AccessManager.contract, event: "TargetAdminDelayUpdated", logs: logs, sub: sub}, nil
}

// WatchTargetAdminDelayUpdated is a free log subscription operation binding the contract event 0xa56b76017453f399ec2327ba00375dbfb1fd070ff854341ad6191e6a2e2de19c.
//
// Solidity: event TargetAdminDelayUpdated(address indexed target, uint32 delay, uint48 since)
func (_AccessManager *AccessManagerFilterer) WatchTargetAdminDelayUpdated(opts *bind.WatchOpts, sink chan<- *AccessManagerTargetAdminDelayUpdated, target []common.Address) (event.Subscription, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "TargetAdminDelayUpdated", targetRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerTargetAdminDelayUpdated)
				if err := _AccessManager.contract.UnpackLog(event, "TargetAdminDelayUpdated", log); err != nil {
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

// ParseTargetAdminDelayUpdated is a log parse operation binding the contract event 0xa56b76017453f399ec2327ba00375dbfb1fd070ff854341ad6191e6a2e2de19c.
//
// Solidity: event TargetAdminDelayUpdated(address indexed target, uint32 delay, uint48 since)
func (_AccessManager *AccessManagerFilterer) ParseTargetAdminDelayUpdated(log types.Log) (*AccessManagerTargetAdminDelayUpdated, error) {
	event := new(AccessManagerTargetAdminDelayUpdated)
	if err := _AccessManager.contract.UnpackLog(event, "TargetAdminDelayUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerTargetClosedIterator is returned from FilterTargetClosed and is used to iterate over the raw logs and unpacked data for TargetClosed events raised by the AccessManager contract.
type AccessManagerTargetClosedIterator struct {
	Event *AccessManagerTargetClosed // Event containing the contract specifics and raw log

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
func (it *AccessManagerTargetClosedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerTargetClosed)
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
		it.Event = new(AccessManagerTargetClosed)
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
func (it *AccessManagerTargetClosedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerTargetClosedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerTargetClosed represents a TargetClosed event raised by the AccessManager contract.
type AccessManagerTargetClosed struct {
	Target common.Address
	Closed bool
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTargetClosed is a free log retrieval operation binding the contract event 0x90d4e7bb7e5d933792b3562e1741306f8be94837e1348dacef9b6f1df56eb138.
//
// Solidity: event TargetClosed(address indexed target, bool closed)
func (_AccessManager *AccessManagerFilterer) FilterTargetClosed(opts *bind.FilterOpts, target []common.Address) (*AccessManagerTargetClosedIterator, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "TargetClosed", targetRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerTargetClosedIterator{contract: _AccessManager.contract, event: "TargetClosed", logs: logs, sub: sub}, nil
}

// WatchTargetClosed is a free log subscription operation binding the contract event 0x90d4e7bb7e5d933792b3562e1741306f8be94837e1348dacef9b6f1df56eb138.
//
// Solidity: event TargetClosed(address indexed target, bool closed)
func (_AccessManager *AccessManagerFilterer) WatchTargetClosed(opts *bind.WatchOpts, sink chan<- *AccessManagerTargetClosed, target []common.Address) (event.Subscription, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "TargetClosed", targetRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerTargetClosed)
				if err := _AccessManager.contract.UnpackLog(event, "TargetClosed", log); err != nil {
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

// ParseTargetClosed is a log parse operation binding the contract event 0x90d4e7bb7e5d933792b3562e1741306f8be94837e1348dacef9b6f1df56eb138.
//
// Solidity: event TargetClosed(address indexed target, bool closed)
func (_AccessManager *AccessManagerFilterer) ParseTargetClosed(log types.Log) (*AccessManagerTargetClosed, error) {
	event := new(AccessManagerTargetClosed)
	if err := _AccessManager.contract.UnpackLog(event, "TargetClosed", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// AccessManagerTargetFunctionRoleUpdatedIterator is returned from FilterTargetFunctionRoleUpdated and is used to iterate over the raw logs and unpacked data for TargetFunctionRoleUpdated events raised by the AccessManager contract.
type AccessManagerTargetFunctionRoleUpdatedIterator struct {
	Event *AccessManagerTargetFunctionRoleUpdated // Event containing the contract specifics and raw log

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
func (it *AccessManagerTargetFunctionRoleUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(AccessManagerTargetFunctionRoleUpdated)
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
		it.Event = new(AccessManagerTargetFunctionRoleUpdated)
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
func (it *AccessManagerTargetFunctionRoleUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *AccessManagerTargetFunctionRoleUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// AccessManagerTargetFunctionRoleUpdated represents a TargetFunctionRoleUpdated event raised by the AccessManager contract.
type AccessManagerTargetFunctionRoleUpdated struct {
	Target   common.Address
	Selector [4]byte
	RoleId   uint64
	Raw      types.Log // Blockchain specific contextual infos
}

// FilterTargetFunctionRoleUpdated is a free log retrieval operation binding the contract event 0x9ea6790c7dadfd01c9f8b9762b3682607af2c7e79e05a9f9fdf5580dde949151.
//
// Solidity: event TargetFunctionRoleUpdated(address indexed target, bytes4 selector, uint64 indexed roleId)
func (_AccessManager *AccessManagerFilterer) FilterTargetFunctionRoleUpdated(opts *bind.FilterOpts, target []common.Address, roleId []uint64) (*AccessManagerTargetFunctionRoleUpdatedIterator, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}

	logs, sub, err := _AccessManager.contract.FilterLogs(opts, "TargetFunctionRoleUpdated", targetRule, roleIdRule)
	if err != nil {
		return nil, err
	}
	return &AccessManagerTargetFunctionRoleUpdatedIterator{contract: _AccessManager.contract, event: "TargetFunctionRoleUpdated", logs: logs, sub: sub}, nil
}

// WatchTargetFunctionRoleUpdated is a free log subscription operation binding the contract event 0x9ea6790c7dadfd01c9f8b9762b3682607af2c7e79e05a9f9fdf5580dde949151.
//
// Solidity: event TargetFunctionRoleUpdated(address indexed target, bytes4 selector, uint64 indexed roleId)
func (_AccessManager *AccessManagerFilterer) WatchTargetFunctionRoleUpdated(opts *bind.WatchOpts, sink chan<- *AccessManagerTargetFunctionRoleUpdated, target []common.Address, roleId []uint64) (event.Subscription, error) {

	var targetRule []interface{}
	for _, targetItem := range target {
		targetRule = append(targetRule, targetItem)
	}

	var roleIdRule []interface{}
	for _, roleIdItem := range roleId {
		roleIdRule = append(roleIdRule, roleIdItem)
	}

	logs, sub, err := _AccessManager.contract.WatchLogs(opts, "TargetFunctionRoleUpdated", targetRule, roleIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(AccessManagerTargetFunctionRoleUpdated)
				if err := _AccessManager.contract.UnpackLog(event, "TargetFunctionRoleUpdated", log); err != nil {
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

// ParseTargetFunctionRoleUpdated is a log parse operation binding the contract event 0x9ea6790c7dadfd01c9f8b9762b3682607af2c7e79e05a9f9fdf5580dde949151.
//
// Solidity: event TargetFunctionRoleUpdated(address indexed target, bytes4 selector, uint64 indexed roleId)
func (_AccessManager *AccessManagerFilterer) ParseTargetFunctionRoleUpdated(log types.Log) (*AccessManagerTargetFunctionRoleUpdated, error) {
	event := new(AccessManagerTargetFunctionRoleUpdated)
	if err := _AccessManager.contract.UnpackLog(event, "TargetFunctionRoleUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
