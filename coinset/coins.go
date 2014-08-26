package coinset

import (
	"errors"
	"sort"

	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

var (
	// ErrCoinsNoSelectionAvailable is returned when a CoinSelector
	// was unable to find any combination of coins which meet the
	// selection requirements.
	ErrCoinsNoSelectionAvailable = errors.New("no coin selection possible")
)

// Coin represents a spendable transaction outpoint
type Coin interface {
	Amount() btcutil.Amount
	ValueAge() int64
}

// Coins represents a set of Coins
type Coins interface {
	Coin(int) Coin
	AmountCoin(int) AmountCoin
	ValueAgeCoin(int) ValueAgeCoin
	Len() int
	Swap(int, int)
}

// AmountCoins represents an ordered, indexed, and reorderable set of
// transaction outputs with known amounts.
type AmountCoins interface {
	AmountCoin(int) AmountCoin
	Len() int
	Swap(int, int)
}

// AmountCoin represents a transaction output with a known amount.
type AmountCoin interface {
	Amount() btcutil.Amount
}

type ValueAgeCoin interface {
	Amount() btcutil.Amount
	ValueAge() int64
}

type ValueAgeCoins interface {
	ValueAgeCoin(i int) ValueAgeCoin
	Len() int
	Swap(int, int)
}

type indexedCoin struct {
	coin  Coin
	index int
}

// It is important to note that the all the Coins being added or removed
// from a set must have a constant ValueAge() during the use of
// the CoinSet, otherwise the cached values will be incorrect.
type subset struct {
	set ValueAgeCoins
	idxs []int
	totalValue    btcutil.Amount
	totalValueAge int64
}

// newSubset creates a subset of a coin set, pushing each coin of the coins from
// the initial coin set.
func newSubset(coins Coins, idxs []int) *subset {
	s := &subset{
		set: coins,
		idxs: idxs,
	}
	for _, i := range idxs {
		coin := coins.Coin(i)
		s.totalValue += coin.Amount()
		s.totalValueAge += coin.ValueAge()
	}
	return s
}

// pushBack adds a coin at some index from another coinset to the end of the
// ordered set.
func (s *subset) pushBack(i int) {
	s.idxs = append(s.idxs, i)
	c := s.set.ValueAgeCoin(i)
	s.totalValue += c.Amount()
	s.totalValueAge += c.ValueAge()
}

// popBack returns and removes the last coin from the ordered set.
// TODO: how to handle empty set?
func (s *subset) popBack() ValueAgeCoin {
	i := s.idxs[len(s.idxs) - 1]
	back := s.set.ValueAgeCoin(i)
	s.idxs = s.idxs[:len(s.idxs)-1]
	s.subTotals(back)
	return back
}

// popFront returns and removes the first coin from the ordered set.
// TODO: how to handle empty set?
func (s *subset) popFront() ValueAgeCoin {
	i := s.idxs[0]
	s.idxs = s.idxs[1:]
	front := s.set.ValueAgeCoin(i)
	s.subTotals(front)
	return front
}

// subTotals subtracts the value amount for a removed coin from the
// cached amounts in the set.
func (s *subset) subTotals(c Coin) {
	s.totalValue -= c.Amount()
	s.totalValueAge -= c.ValueAge()
}

// indexes returns the indexes of all coins from the coinset that were added
// to the utility set.
func (s *subset) indexes() []int {
	return s.idxs
}

// satisfiesTargetAmount returns whether the total amount is either exactly the
// target or is greater than the target by at least the minChange amount.
func satisfiesTargetAmount(target, minChange, total btcutil.Amount) bool {
	return total == target || total >= target+minChange
}

// Selector is an interface that wraps the CoinSelect method.
//
// Select will attempt to select a subset of the coins which has at least the
// targetValue amount.  CoinSelect is not guaranteed to return a
// selection of coins even if the total value of coins given is greater
// than the target value.
//
// The exact choice of coins in the subset will be implementation specific.
//
// It is important to note that the Coins being used as inputs need to have
// a constant ValueAge() during the execution of CoinSelect.
type Selector interface {
	CoinSelect(targetValue btcutil.Amount, coins []Coin) (Coins, error)
}

// Args describes the common arguments used by all selection algorithms when
// selecting previous outputs to use in a new transaction.
type Args struct {
	MaxInputs       int
	MinChangeAmount btcutil.Amount
}

// SelectMinIndex will attempt to construct a coin selection whose total value
// is at least target and prefers any number of lower indexes (as in the
// ordered array or slice) over higher ones.
func (a Args) SelectMinIndex(target btcutil.Amount, coins AmountCoins) ([]int, error) {
	sel := make([]AmountCoin, 0, a.MaxInputs)
	var total btcutil.Amount

	numCoins := coins.Len()
	for i := 0; i < numCoins && i < a.MaxInputs; i++ {
		coin := coins.AmountCoin(i)
		sel = append(sel, coin)
		total += coin.Amount()
		if satisfiesTargetAmount(target, a.MinChangeAmount, total) {
			idxs := make([]int, len(sel))
			for i := range sel {
				idxs[i] = i
			}
			return idxs, nil
		}
	}
	return nil, ErrCoinsNoSelectionAvailable
}

// MinNumberCoinSelector is a CoinSelector that attempts to construct
// a selection of coins whose total value is at least targetValue
// that uses as few of the inputs as possible.
type MinNumberCoinSelector struct {
	MaxInputs       int
	MinChangeAmount btcutil.Amount
}

// MinNumberSelect will attempt to construct a coin selection whose total
// value is at least targetValue using at few inputs as possible.
func (a Args) MinNumberSelect(target btcutil.Amount, coins AmountCoins) ([]int, error) {
	sort.Sort(sort.Reverse(byAmount{coins}))
	return a.SelectMinIndex(target, coins)
}

// MaxValueAgeSelect will attempt to construct a coin selection whose total
// value is at least target and has as much input value-age as possible.
//
// This would be useful in the case where you want to maximize likelihood
// of the inclusion of your transaction in the next mined block.
func (a Args) MaxValueAgeSelect(target btcutil.Amount, coins Coins) ([]int, error) {
	sort.Sort(sort.Reverse(byValueAge{coins}))
	return a.SelectMinIndex(target, coins)
}

// MinPrioritySelect will attempt to construct a coin selection whose total
// total amount is at least target and whose average value-age per input is
// greater than minAvgValueAgePerInput.
//
// When possible, MinPrioritySelect will attempt to reduce the average input
// priority over the threshold, but no guarantees will be made as to minimality
// of the selection.  The selection below is almost certainly suboptimal.
func (a Args) MinPrioritySelect(minAvgValueAgePerInput int64, target btcutil.Amount, coins ValueAgeCoins) ([]int, error) {
	sort.Sort(byValueAge{coins})

	// find the first coin with sufficient valueAge
	cutoffIndex := -1
	numCoins := coins.Len()
	for i := 0; i < numCoins; i++ {
		if coins.ValueAgeCoin(i).ValueAge() >= minAvgValueAgePerInput {
			cutoffIndex = i
			break
		}
	}
	if cutoffIndex < 0 {
		return nil, ErrCoinsNoSelectionAvailable
	}

	// create sets of input coins that will obey minimum average valueAge
	for i := cutoffIndex; i < numCoins; i++ {
		possibleHighCoins := coins[cutoffIndex : i+1]

		// choose a set of high-enough valueAge coins
		highSelect, err := a.MinNumberSelect(targetValue, possibleHighCoins)
		if err != nil {
			// attempt to add available low priority to make a solution

			for numLow := 1; numLow <= cutoffIndex && numLow+(i-cutoffIndex) <= a.MaxInputs; numLow++ {
				allHigh := NewCoinSet(coins[cutoffIndex : i+1])
				newTargetValue := targetValue - allHigh.TotalValue()
				newMaxInputs := allHigh.Num() + numLow
				if newMaxInputs > numLow {
					newMaxInputs = numLow
				}
				newMinAvgValueAge := ((minAvgValueAgePerInput * int64(allHigh.Num()+numLow)) - allHigh.TotalValueAge()) / int64(numLow)

				// find the minimum priority that can be added to set
				lowSelect, err := Args{
					MaxInputs:       newMaxInputs,
					MinChangeAmount: a.MinChangeAmount,
				}.MinPrioritySelect(newMinAvgValueAge, newTargetVAlue, coins[0:cutoffIndex])

				if err != nil {
					continue
				}

				for _, coin := range lowSelect.Coins() {
					allHigh.PushCoin(coin)
				}

				return allHigh, nil
			}
			// oh well, couldn't fix, try to add more high priority to the set.
		} else {
			extendedCoins := NewCoinSet(highSelect.Coins())

			// attempt to lower priority towards target with lowest ones first
			for n := 0; n < cutoffIndex; n++ {
				if extendedCoins.Num() >= s.MaxInputs {
					break
				}
				if coins[n].ValueAge() == 0 {
					continue
				}

				extendedCoins.PushCoin(coins[n])
				if extendedCoins.TotalValueAge()/int64(extendedCoins.Num()) < s.MinAvgValueAgePerInput {
					extendedCoins.PopCoin()
					continue
				}
			}
			return extendedCoins, nil
		}
	}

	return nil, ErrCoinsNoSelectionAvailable
}

type byValueAge struct {
	ValueAgeCoins
}

func (b byValueAge) Less(i, j int) bool {
	return b.ValueAgeCoins.ValueAgeCoin(i).ValueAge() < b.ValueAgeCoins.ValueAgeCoin(j).ValueAge()
}

type byAmount struct {
	AmountCoins
}

func (b byAmount) Less(i, j int) bool {
	return b.AmountCoins.AmountCoin(i).Amount() < b.AmountCoins.AmountCoin(j).Amount()
}

// SimpleCoin defines a concrete instance of Coin that is backed by a
// btcutil.Tx, a specific outpoint index, and the number of confirmations
// that transaction has had.
type SimpleCoin struct {
	Tx         *btcutil.Tx
	Output    uint32
	TxNumConfs int64
}

// Ensure that SimpleCoin is a Coin
var _ Coin = (*SimpleCoin)(nil)

// txOut returns the TxOut of the transaction the Coin represents
func (c *SimpleCoin) txOut() *btcwire.TxOut {
	return c.Tx.MsgTx().TxOut[c.Output]
}

// Value returns the value of the Coin
func (c *SimpleCoin) Amount() btcutil.Amount {
	return btcutil.Amount(c.txOut().Amount)
}

// ValueAge returns the product of the value and the number of confirmations.  This is
// used as an input to calculate the priority of the transaction.
func (c *SimpleCoin) ValueAge() int64 {
	return c.TxNumConfs * int64(c.Amount())
}
