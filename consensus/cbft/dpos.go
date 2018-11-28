package cbft

import (
	"Platon-go/common"
	"Platon-go/core"
	"Platon-go/core/dpos"
	"Platon-go/core/state"
	"Platon-go/core/types"
	"Platon-go/core/vm"
	"Platon-go/log"
	"Platon-go/p2p/discover"
	"Platon-go/params"
	"math/big"
	"sync"
)

type dpos struct {
	former            *dposRound // the previous round of witnesses nodeId
	current           *dposRound // the current round of witnesses nodeId
	next              *dposRound // the next round of witnesses nodeId
	chain             *core.BlockChain
	lastCycleBlockNum uint64
	startTimeOfEpoch  int64 // 一轮共识开始时间，通常是上一轮共识结束时最后一个区块的出块时间；如果是第一轮，则从1970.1.1.0.0.0.0开始。单位：秒
	config            *params.DposConfig

	// added by candidatepool module

	lock sync.RWMutex
	// the candidate pool object pointer
	candidatePool *depos.CandidatePool
}

type dposRound struct {
	nodes []discover.NodeID
	start *big.Int
	end   *big.Int
}

func newDpos(initialNodes []discover.NodeID, config *params.CbftConfig) *dpos {

	formerRound := &dposRound{
		nodes: initialNodes,
		start: big.NewInt(1),
		end:   big.NewInt(BaseSwitchWitness),
	}
	currentRound := &dposRound{
		nodes: initialNodes,
		start: big.NewInt(1),
		end:   big.NewInt(BaseSwitchWitness),
	}
	return &dpos{
		former:            formerRound,
		current:           currentRound,
		lastCycleBlockNum: 0,
		config:            config.DposConfig,
		candidatePool:     depos.NewCandidatePool(config.DposConfig),
	}
	//return dposPtr
}

func (d *dpos) AnyIndex(nodeID discover.NodeID) int64 {
	d.lock.RLock()
	defer d.lock.RUnlock()
	nodeList := make([]discover.NodeID, 0)
	if d.former != nil && d.former.nodes != nil && len(d.former.nodes) > 0 {
		nodeList = append(nodeList, d.former.nodes...)
	}
	if d.current != nil && d.current.nodes != nil && len(d.current.nodes) > 0 {
		nodeList = append(nodeList, d.current.nodes...)
	}
	if d.next != nil && d.next.nodes != nil && len(d.next.nodes) > 0 {
		nodeList = append(nodeList, d.next.nodes...)
	}
	for idx, node := range nodeList {
		if node == nodeID {
			return int64(idx)
		}
	}
	return int64(-1)
}

func (d *dpos) BlockProducerIndex(number uint64, nodeID discover.NodeID) int64 {
	d.lock.RLock()
	defer d.lock.RUnlock()
	if number == 0 {
		for idx, node := range d.current.nodes {
			if node == nodeID {
				return int64(idx)
			}
		}
		return -1
	}
	if number >= d.former.start.Uint64() && number <= d.former.end.Uint64() {
		for idx, node := range d.former.nodes {
			if node == nodeID {
				return int64(idx)
			}
		}
		return -1
	}

	if d.next != nil && number >= d.next.start.Uint64() && number <= d.next.end.Uint64() {
		for idx, node := range d.next.nodes {
			if node == nodeID {
				return int64(idx)
			}
		}
		return -1
	}
	return -1

}

func (d *dpos) NodeIndexInFuture(nodeID discover.NodeID) int64 {
	d.lock.RLock()
	defer d.lock.RUnlock()
	nodeList := append(d.current.nodes, d.next.nodes...)
	for idx, node := range nodeList {
		if node == nodeID {
			return int64(idx)
		}
	}
	return int64(-1)
}

func (d *dpos) getCurrentNodes() []discover.NodeID {
	d.lock.RLock()
	defer d.lock.RUnlock()
	return d.current.nodes
}

func (d *dpos) consensusNodes(blockNum *big.Int) []discover.NodeID {
	d.lock.RLock()
	defer d.lock.RUnlock()

	if d.former != nil && blockNum.Cmp(d.former.start) >= 0 && blockNum.Cmp(d.former.end) <= 0 {
		return d.former.nodes
	} else if d.current != nil && blockNum.Cmp(d.current.start) >= 0 && blockNum.Cmp(d.current.end) <= 0 {
		return d.current.nodes
	} else if d.next != nil && blockNum.Cmp(d.next.start) >= 0 && blockNum.Cmp(d.next.end) <= 0 {
		return d.next.nodes
	}
	return nil
}

func (d *dpos) LastCycleBlockNum() uint64 {
	// 获取最后一轮共识结束时的区块高度
	return d.lastCycleBlockNum
}

func (d *dpos) SetLastCycleBlockNum(blockNumber uint64) {
	// 设置最后一轮共识结束时的区块高度
	d.lastCycleBlockNum = blockNumber
}

// modify by platon
// 返回当前共识节点地址列表
/*func (b *dpos) ConsensusNodes() []discover.Node {
	return b.primaryNodeList
}
*/
// 判断某个节点是否本轮或上一轮选举共识节点
/*func (b *dpos) CheckConsensusNode(id discover.NodeID) bool {
	nodes := b.ConsensusNodes()
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}*/

// 判断当前节点是否本轮或上一轮选举共识节点
/*func (b *dpos) IsConsensusNode() (bool, error) {
	return true, nil
}
*/

func (d *dpos) StartTimeOfEpoch() int64 {
	return d.startTimeOfEpoch
}

func (d *dpos) SetStartTimeOfEpoch(startTimeOfEpoch int64) {
	// 设置最后一轮共识结束时的出块时间
	d.startTimeOfEpoch = startTimeOfEpoch
	log.Info("设置最后一轮共识结束时的出块时间", "startTimeOfEpoch", startTimeOfEpoch)
}

/** dpos was added func */
/** Method provided to the cbft module call */
// Announce witness
func (d *dpos) Election(state *state.StateDB, blocknumber *big.Int) ([]*discover.Node, error) {
	if nextNodes, err := d.candidatePool.Election(state); nil != err {
		log.Error("dpos election next witness err", err)
		panic("Election error " + err.Error())
	} else {
		log.Info("揭榜完成，再次查看stateDB信息...")
		d.candidatePool.GetAllWitness(state)
		// current round
		round := calcurround(blocknumber)

		d.lock.Lock()
		nextStart := big.NewInt(int64(BaseSwitchWitness*(round+1)) + 1)
		nextEnd := new(big.Int).Add(nextStart, big.NewInt(int64(BaseSwitchWitness-1)))
		d.next = &dposRound{
			nodes: convertNodeID(nextNodes),
			start: nextStart,
			end:   nextEnd,
		}

		log.Info("揭榜维护下一轮的nodeIds长度:", "len", len(nextNodes))
		depos.PrintObject("揭榜维护下一轮的nodeIds:", nextNodes)
		depos.PrintObject("揭榜的上轮dposRound：", d.former.nodes)
		depos.PrintObject("揭榜的当前轮dposRound：", d.current.nodes)
		depos.PrintObject("揭榜维护下一轮dposRound：", d.next.nodes)
		d.lock.Unlock()
		return nextNodes, nil
	}
}

// switch next witnesses to current witnesses
func (d *dpos) Switch(state *state.StateDB /*, start, end *big.Int*/) bool {
	log.Info("Switch begin...")
	if !d.candidatePool.Switch(state) {
		return false
	}
	log.Info("Switch success...")
	preArr, curArr, _, err := d.candidatePool.GetAllWitness(state)
	if nil != err {
		return false
	}
	d.lock.Lock()
	if len(preArr) != 0 {
		d.former = &dposRound{
			nodes: convertNodeID(preArr),
			start: d.current.start,
			end:   d.current.end,
		}
	}
	if len(curArr) != 0 {
		d.current = &dposRound{
			nodes: convertNodeID(curArr),
			start: d.next.start,
			end:   d.next.end,
		}
	}
	d.next = nil
	depos.PrintObject("Switch获取上一轮nodes：", preArr)
	depos.PrintObject("Switch获取上当前轮nodes：", curArr)
	depos.PrintObject("Switch的上轮dposRound：", d.former.nodes)
	depos.PrintObject("Switch的当前轮dposRound：", d.current.nodes)

	d.lock.Unlock()
	return true
}

// Getting nodes of witnesses
// flag：-1: the previous round of witnesses  0: the current round of witnesses   1: the next round of witnesses
func (d *dpos) GetWitness(state *state.StateDB, flag int) ([]*discover.Node, error) {
	return d.candidatePool.GetWitness(state, flag)
}

func (d *dpos) GetAllWitness(state *state.StateDB) ([]*discover.Node, []*discover.Node, []*discover.Node, error) {
	return d.candidatePool.GetAllWitness(state)
}

// setting candidate pool of dpos module
func (d *dpos) SetCandidatePool(blockChain *core.BlockChain) {
	// When the highest block in the chain is not a genesis block, Need to load witness nodeIdList from the stateDB.
	if blockChain.Genesis().NumberU64() != blockChain.CurrentBlock().NumberU64() {
		state, err := blockChain.State()
		log.Warn("---重新启动节点，更新formerlyNodeList、primaryNodeList和nextNodeList---", "state", state)
		if nil != err {
			log.Error("Load state from chain failed on SetCandidatePool err", err)
			return
		}
		if preArr, curArr, nextArr, err := d.candidatePool.GetAllWitness(state); nil != err {
			log.Error("Load Witness from state failed on SetCandidatePool err", err)
		} else {
			d.lock.Lock()
			// current round
			round := calcurround(blockChain.CurrentBlock().Number())

			d.former.start = big.NewInt(int64(BaseSwitchWitness*(round-1)) + 1)
			d.former.end = new(big.Int).Add(d.former.start, big.NewInt(int64(BaseSwitchWitness-1)))
			if len(preArr) != 0 {
				d.former.nodes = convertNodeID(preArr)
			}

			d.current.start = big.NewInt(int64(BaseSwitchWitness*round) + 1)
			d.current.end = new(big.Int).Add(d.current.start, big.NewInt(int64(BaseSwitchWitness-1)))
			if len(curArr) != 0 {
				d.current.nodes = convertNodeID(curArr)
			}
			if len(nextArr) != 0 {
				start := big.NewInt(int64(BaseSwitchWitness*(round+1)) + 1)
				end := new(big.Int).Add(start, big.NewInt(int64(BaseSwitchWitness-1)))
				d.next = &dposRound{
					nodes: 		convertNodeID(nextArr),
					start: 		start,
					end: 		end,
				}
				depos.PrintObject("重新加载获取上当前轮nodes：", nextArr)
				depos.PrintObject("重新加载的上轮dposRound：", d.next.nodes)
			}
			depos.PrintObject("重新加载获取上一轮nodes：", preArr)
			depos.PrintObject("重新加载获取上当前轮nodes：", curArr)
			depos.PrintObject("重新加载的上轮dposRound：", d.former.nodes)
			depos.PrintObject("重新加载的当前轮dposRound：", d.current.nodes)

			d.lock.Unlock()
		}
	}
}

/** Method provided to the built-in contract call */
// pledge Candidate
func (d *dpos) SetCandidate(state vm.StateDB, nodeId discover.NodeID, can *types.Candidate) error {
	return d.candidatePool.SetCandidate(state, nodeId, can)
}

// Getting immediate candidate info by nodeId
func (d *dpos) GetCandidate(state vm.StateDB, nodeId discover.NodeID) (*types.Candidate, error) {
	return d.candidatePool.GetCandidate(state, nodeId)
}

// candidate withdraw from immediates elected candidates
func (d *dpos) WithdrawCandidate(state vm.StateDB, nodeId discover.NodeID, price, blockNumber *big.Int) error {
	return d.candidatePool.WithdrawCandidate(state, nodeId, price, blockNumber)
}

// Getting all immediate elected candidates array
func (d *dpos) GetChosens(state vm.StateDB) []*types.Candidate {
	return d.candidatePool.GetChosens(state)
}

// Getting all witness array
func (d *dpos) GetChairpersons(state vm.StateDB) []*types.Candidate {
	return d.candidatePool.GetChairpersons(state)
}

// Getting all refund array by nodeId
func (d *dpos) GetDefeat(state vm.StateDB, nodeId discover.NodeID) ([]*types.Candidate, error) {
	return d.candidatePool.GetDefeat(state, nodeId)
}

// Checked current candidate was defeat by nodeId
func (d *dpos) IsDefeat(state vm.StateDB, nodeId discover.NodeID) (bool, error) {
	return d.candidatePool.IsDefeat(state, nodeId)
}

// refund once
func (d *dpos) RefundBalance(state vm.StateDB, nodeId discover.NodeID, blockNumber *big.Int) error {
	return d.candidatePool.RefundBalance(state, nodeId, blockNumber)
}

// Getting owner's address of candidate info by nodeId
func (d *dpos) GetOwner(state vm.StateDB, nodeId discover.NodeID) common.Address {
	return d.candidatePool.GetOwner(state, nodeId)
}

// Getting allow block interval for refunds
func (d *dpos) GetRefundInterval() uint64 {
	return d.candidatePool.GetRefundInterval()
}

// cbft共识区块产生分叉后需要更新primaryNodeList和formerlyNodeList
func (d *dpos) UpdateNodeList(state *state.StateDB, blocknumber *big.Int) {
	log.Warn("---cbft共识区块产生分叉，更新formerlyNodeList、primaryNodeList和nextNodeList---", "state", state)
	if preArr, curArr, _, err := d.candidatePool.GetAllWitness(state); nil != err {
		log.Error("Load Witness from state failed on UpdateNodeList err", err)
		panic("UpdateNodeList error")
	} else {
		d.lock.Lock()

		// current round
		round := calcurround(blocknumber)

		start := big.NewInt(int64(BaseSwitchWitness*round) + 1)
		end := new(big.Int).Add(d.current.start, big.NewInt(int64(BaseSwitchWitness-1)))

		formerStartReset := new(big.Int).Sub(start, new(big.Int).SetUint64(uint64(BaseSwitchWitness)))
		formerEndReset := new(big.Int).Sub(end, new(big.Int).SetUint64(uint64(BaseSwitchWitness)))
		if len(preArr) != 0 {
			d.former = &dposRound{
				nodes: convertNodeID(preArr),
				start: formerStartReset,
				end:   formerEndReset,
			}
		}
		if len(curArr) != 0 {
			d.current = &dposRound{
				nodes: convertNodeID(curArr),
				start: start,
				end:   end,
			}
		}
		d.next = nil
		depos.PrintObject("分叉获取上一轮nodes：", preArr)
		depos.PrintObject("分叉获取上当前轮nodes：", curArr)
		depos.PrintObject("分叉的上轮dposRound：", d.former.nodes)
		depos.PrintObject("分叉的当前轮dposRound：", d.current.nodes)
		d.lock.Unlock()
	}
}

func convertNodeID(nodes []*discover.Node) []discover.NodeID {
	nodesID := make([]discover.NodeID, 0, len(nodes))
	for _, n := range nodes {
		nodesID = append(nodesID, n.ID)
	}
	return nodesID
}

// calculate current round number by current blocknumber
func calcurround(blocknumber *big.Int) uint64 {
	// current num
	var round uint64
	div := blocknumber.Uint64() / BaseSwitchWitness
	mod := blocknumber.Uint64() % BaseSwitchWitness
	if (div == 0 && mod == 0) || (div == 0 && mod > 0 && mod < BaseSwitchWitness) { // first round
		round = 1
	} else if div > 0 && mod == 0 {
		round = div
	} else if div > 0 && mod > 0 && mod < BaseSwitchWitness {
		round = div + 1
	}
	return round
}

//func (d *dpos) MaxChair() int64 {
//	return int64(d.candidatePool.MaxChair())
//}
