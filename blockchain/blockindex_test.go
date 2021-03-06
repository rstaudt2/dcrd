// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/wire"
)

// TestBlockNodeHeader ensures that block nodes reconstruct the correct header
// and fetching the header from the chain reconstructs it from memory.
func TestBlockNodeHeader(t *testing.T) {
	// Create a fake chain and block header with all fields set to nondefault
	// values.
	params := chaincfg.RegNetParams()
	bc := newFakeChain(params)
	tip := bc.bestChain.Tip()
	testHeader := wire.BlockHeader{
		Version:      1,
		PrevBlock:    tip.hash,
		MerkleRoot:   *mustParseHash("09876543210987654321"),
		StakeRoot:    *mustParseHash("43210987654321098765"),
		VoteBits:     0x03,
		FinalState:   [6]byte{0xaa},
		Voters:       4,
		FreshStake:   5,
		Revocations:  6,
		PoolSize:     20000,
		Bits:         0x1234,
		SBits:        123456789,
		Height:       1,
		Size:         393216,
		Timestamp:    time.Unix(1454954400, 0),
		Nonce:        7,
		ExtraData:    [32]byte{0xbb},
		StakeVersion: 5,
	}
	node := newBlockNode(&testHeader, tip)
	bc.index.AddNode(node)

	// Ensure reconstructing the header for the node produces the same header
	// used to create the node.
	gotHeader := node.Header()
	if !reflect.DeepEqual(gotHeader, testHeader) {
		t.Fatalf("node.Header: mismatched headers: got %+v, want %+v",
			gotHeader, testHeader)
	}

	// Ensure fetching the header from the chain produces the same header used
	// to create the node.
	testHeaderHash := testHeader.BlockHash()
	gotHeader, err := bc.HeaderByHash(&testHeaderHash)
	if err != nil {
		t.Fatalf("HeaderByHash: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(gotHeader, testHeader) {
		t.Fatalf("HeaderByHash: mismatched headers: got %+v, want %+v",
			gotHeader, testHeader)
	}
}

// TestCalcPastMedianTime ensures the CalcPastMedianTie function works as
// intended including when there are less than the typical number of blocks
// which happens near the beginning of the chain.
func TestCalcPastMedianTime(t *testing.T) {
	tests := []struct {
		name       string
		timestamps []int64
		expected   int64
	}{
		{
			name:       "one block",
			timestamps: []int64{1517188771},
			expected:   1517188771,
		},
		{
			name:       "two blocks, in order",
			timestamps: []int64{1517188771, 1517188831},
			expected:   1517188771,
		},
		{
			name:       "three blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891},
			expected:   1517188831,
		},
		{
			name:       "three blocks, out of order",
			timestamps: []int64{1517188771, 1517188891, 1517188831},
			expected:   1517188831,
		},
		{
			name:       "four blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891, 1517188951},
			expected:   1517188831,
		},
		{
			name:       "four blocks, out of order",
			timestamps: []int64{1517188831, 1517188771, 1517188951, 1517188891},
			expected:   1517188831,
		},
		{
			name: "eleven blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891, 1517188951,
				1517189011, 1517189071, 1517189131, 1517189191, 1517189251,
				1517189311, 1517189371},
			expected: 1517189071,
		},
		{
			name: "eleven blocks, out of order",
			timestamps: []int64{1517188831, 1517188771, 1517188891, 1517189011,
				1517188951, 1517189071, 1517189131, 1517189191, 1517189251,
				1517189371, 1517189311},
			expected: 1517189071,
		},
		{
			name: "fifteen blocks, in order",
			timestamps: []int64{1517188771, 1517188831, 1517188891, 1517188951,
				1517189011, 1517189071, 1517189131, 1517189191, 1517189251,
				1517189311, 1517189371, 1517189431, 1517189491, 1517189551,
				1517189611},
			expected: 1517189311,
		},
		{
			name: "fifteen blocks, out of order",
			timestamps: []int64{1517188771, 1517188891, 1517188831, 1517189011,
				1517188951, 1517189131, 1517189071, 1517189251, 1517189191,
				1517189371, 1517189311, 1517189491, 1517189431, 1517189611,
				1517189551},
			expected: 1517189311,
		},
	}

	// Ensure the genesis block timestamp of the test params is before the test
	// data.  Also, clone the provided parameters first to avoid mutating them.
	//
	// The timestamp corresponds to 2018-01-01 00:00:00 +0000 UTC.
	params := chaincfg.RegNetParams()
	params.GenesisBlock.Header.Timestamp = time.Unix(1514764800, 0)
	params.GenesisHash = params.GenesisBlock.BlockHash()

	for _, test := range tests {
		// Create a synthetic chain with the correct number of nodes and the
		// timestamps as specified by the test.
		bc := newFakeChain(params)
		node := bc.bestChain.Tip()
		for _, timestamp := range test.timestamps {
			node = newFakeNode(node, 0, 0, 0, time.Unix(timestamp, 0))
			bc.index.AddNode(node)
			bc.bestChain.SetTip(node)
		}

		// Ensure the median time is the expected value.
		gotTime := node.CalcPastMedianTime()
		wantTime := time.Unix(test.expected, 0)
		if !gotTime.Equal(wantTime) {
			t.Errorf("%s: mismatched timestamps -- got: %v, want: %v",
				test.name, gotTime, wantTime)
			continue
		}
	}
}

// TestChainTips ensures the chain tip tracking in the block index works
// as expected.
func TestChainTips(t *testing.T) {
	params := chaincfg.RegNetParams()
	bc := newFakeChain(params)
	genesis := bc.bestChain.NodeByHeight(0)

	// Construct a synthetic simnet chain consisting of the following structure.
	// 0 -> 1 -> 2  -> 3  -> 4
	//  |    \-> 2a -> 3a -> 4a -> 5a -> 6a -> 7a -> ... -> 26a
	//  |    |     \-> 3b -> 4b -> 5b
	//  |    \-> 2c -> 3c -> 4c -> 5c -> 6c -> 7c -> ... -> 26c
	//  \-> 1d
	//  |     \
	//  \-> 1e |
	//         \-> 2f (added after 1e)

	branches := make([][]*blockNode, 7)
	branches[0] = chainedFakeNodes(genesis, 4)
	branches[1] = chainedFakeNodes(branches[0][0], 25)
	branches[2] = chainedFakeNodes(branches[1][0], 3)
	branches[3] = chainedFakeNodes(branches[0][0], 25)
	branches[4] = chainedFakeNodes(genesis, 1)
	branches[5] = chainedFakeNodes(genesis, 1)
	branches[6] = chainedFakeNodes(branches[4][0], 1)

	// Add all of the nodes to the index.
	for _, branch := range branches {
		for _, node := range branch {
			bc.index.AddNode(node)
		}
	}

	// Create a map of all of the chain tips the block index believes exist.
	chainTips := make(map[*blockNode]struct{})
	bc.index.RLock()
	for _, entry := range bc.index.chainTips {
		chainTips[entry.tip] = struct{}{}
		for _, node := range entry.otherTips {
			chainTips[node] = struct{}{}
		}
	}
	bc.index.RUnlock()

	// Exclude tips that are part of an earlier set of branch nodes that was
	// built on via a new set of branch nodes.
	excludeExpected := make(map[*blockNode]struct{})
	excludeExpected[branchTip(branches[4])] = struct{}{}

	// The expected chain tips are the tips of all of the branches minus any
	// that were excluded.
	expectedTips := make(map[*blockNode]struct{})
	for _, branch := range branches {
		if _, ok := excludeExpected[branchTip(branch)]; ok {
			continue
		}
		expectedTips[branchTip(branch)] = struct{}{}
	}

	// Ensure the chain tips are the expected values.
	if len(chainTips) != len(expectedTips) {
		t.Fatalf("block index reports %d chain tips, but %d were expected",
			len(chainTips), len(expectedTips))
	}
	for node := range expectedTips {
		if _, ok := chainTips[node]; !ok {
			t.Fatalf("block index does not contain expected tip %s (height %d)",
				node.hash, node.height)
		}
	}
}

// TestAncestorSkipList ensures the skip list functionality and ancestor
// traversal that makes use of it works as expected.
func TestAncestorSkipList(t *testing.T) {
	// Create fake nodes to use for skip list traversal.
	nodes := chainedFakeSkipListNodes(nil, 250000)

	// Ensure the skip list is constructed correctly by checking that each node
	// points to an ancestor with a lower height and that said ancestor is
	// actually the node at that height.
	for i, node := range nodes[1:] {
		ancestorHeight := node.skipToAncestor.height
		if ancestorHeight >= int64(i+1) {
			t.Fatalf("height for skip list pointer %d is not lower than "+
				"current node height %d", ancestorHeight, int64(i+1))
		}

		if node.skipToAncestor != nodes[ancestorHeight] {
			t.Fatalf("unxpected node for skip list pointer for height %d",
				ancestorHeight)
		}
	}

	// Use a unique random seed each test instance and log it if the tests fail.
	seed := time.Now().Unix()
	rng := rand.New(rand.NewSource(seed))
	defer func(t *testing.T, seed int64) {
		if t.Failed() {
			t.Logf("random seed: %d", seed)
		}
	}(t, seed)

	for i := 0; i < 2500; i++ {
		// Ensure obtaining the ancestor at a random starting height from the
		// tip is the expected node.
		startHeight := rng.Int63n(int64(len(nodes) - 1))
		startNode := nodes[startHeight]
		if branchTip(nodes).Ancestor(startHeight) != startNode {
			t.Fatalf("unxpected ancestor for height %d from tip",
				startHeight)
		}

		// Ensure obtaining the ancestor at height 0 starting from the node at
		// the random starting height is the expected node.
		if startNode.Ancestor(0) != nodes[0] {
			t.Fatalf("unxpected ancestor for height 0 from start height %d",
				startHeight)
		}

		// Ensure obtaining the ancestor from a random ending height starting
		// from the node at the random starting height is the expected node.
		endHeight := rng.Int63n(startHeight + 1)
		if startNode.Ancestor(endHeight) != nodes[endHeight] {
			t.Fatalf("unxpected ancestor for height %d from start height %d",
				endHeight, startHeight)
		}
	}
}

// TestWorkSorterCompare ensures the work sorter less and hash comparison
// functions work as intended including multiple keys.
func TestWorkSorterCompare(t *testing.T) {
	lowerHash := mustParseHash("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba")
	higherHash := mustParseHash("000000000000d41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba")
	tests := []struct {
		name     string     // test description
		nodeA    *blockNode // first node to compare
		nodeB    *blockNode // second node to compare
		wantCmp  int        // expected result of the hash comparison
		wantLess bool       // expected result of the less comparison

	}{{
		name: "exactly equal, both data",
		nodeA: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  0,
		wantLess: false,
	}, {
		name: "exactly equal, no data",
		nodeA: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *mustParseHash("0000000000000000000000000000000000000000000000000000000000000000"),
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  0,
		wantLess: false,
	}, {
		name: "a has more cumulative work, same order, higher hash, a has data",
		nodeA: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(4),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  1,
		wantLess: false,
	}, {
		name: "a has less cumulative work, same order, lower hash, b has data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(4),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: true,
	}, {
		name: "a has same cumulative work, same order, lower hash, a has data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, higher hash, b has data",
		nodeA: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  1,
		wantLess: true,
	}, {
		name: "a has same cumulative work, higher order, lower hash, both data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 1,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: true,
	}, {
		name: "a has same cumulative work, lower order, lower hash, both data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 1,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 2,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, lower hash, no data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, lower hash, both data",
		nodeA: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  -1,
		wantLess: false,
	}, {
		name: "a has same cumulative work, same order, higher hash, both data",
		nodeA: &blockNode{
			hash:            *higherHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		nodeB: &blockNode{
			hash:            *lowerHash,
			workSum:         big.NewInt(2),
			status:          statusDataStored,
			receivedOrderID: 0,
		},
		wantCmp:  1,
		wantLess: true,
	}}

	for _, test := range tests {
		gotLess := workSorterLess(test.nodeA, test.nodeB)
		if gotLess != test.wantLess {
			t.Fatalf("%q: unexpected result -- got %v, want %v", test.name,
				gotLess, test.wantLess)
		}

		gotCmp := compareHashesAsUint256LE(&test.nodeA.hash, &test.nodeB.hash)
		if gotCmp != test.wantCmp {
			t.Fatalf("%q: unexpected result -- got %v, want %v", test.name,
				gotCmp, test.wantCmp)
		}
	}
}
