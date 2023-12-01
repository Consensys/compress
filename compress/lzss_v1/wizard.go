package lzss_v1

import (
	"github.com/consensys/accelerated-crypto-monorepo/maths/common/smartvectors"
	"github.com/consensys/accelerated-crypto-monorepo/maths/field"
	"github.com/consensys/accelerated-crypto-monorepo/protocol/commitment"
	"github.com/consensys/accelerated-crypto-monorepo/protocol/query"
	"github.com/consensys/accelerated-crypto-monorepo/protocol/wizard"
	"github.com/consensys/accelerated-crypto-monorepo/symbolic"
)

// this is an implementation of the lzss_v1 decompressor using the "IOP Wizard"

var (
	InIName      commitment.Name = "IN_I"
	BrOffsetName commitment.Name = "BR_OFFSET"
	BrLengthName commitment.Name = "BR_LENGTH"
	CurrName     commitment.Name = "CURR"
)

func define(b *wizard.Builder, cMaxLen, dMaxLen int, settings Settings) {
	/*inI := b.RegisterCommit(InIName, dMaxLen)
	inINext := inI.Shift(1)

	brOffset := b.RegisterCommit(BrOffsetName, dMaxLen)
	brLength := b.RegisterCommit(BrLengthName, dMaxLen)
	curr := b.RegisterCommit(CurrName, dMaxLen)*/
	//expr_handle.ExprHandle()
}

func prove(r *wizard.ProverRuntime) {
	r.AssignCommitment(InIName, smartvectors.ForTest(1, 2, 3))
}

func IsZero(c *wizard.CompiledIOP, x commitment.Handle) { // TODO see if already implemented
	round := x.Round() // TODO What is round
	inv := c.InsertCommit(round, commitment.Namef("%s_inv", x.GetName()), x.Size())
	m := c.InsertCommit(round, commitment.Namef("%s_isZero", x.GetName()), x.Size())

	c.SubProvers.AppendToInner(round, func(r *wizard.ProverRuntime) {
		xW := r.GetWitness(x.GetName())
		invW := make([]field.Element, x.Size())
		mW := make([]field.Element, x.Size())
		for i := range invW {
			xWI := xW.Get(i)
			invW[i] = xW.Get(i)
			if xWI.IsZero() {
				mW[i].SetOne()
			}
		}
		field.BatchInvert(invW)
		r.AssignCommitment(m.GetName(), smartvectors.NewRegular(mW))
		r.AssignCommitment(inv.GetName(), smartvectors.NewRegular(invW))
	})

	c.InsertGlobal(round, query.Namef("%s_isZero0", x.GetName()), x.AsVariable().Mul(inv.AsVariable()).Add(m.AsVariable()).Sub(symbolic.NewConstant(1)))
	c.InsertGlobal(round, query.Namef("%s_isZero1", x.GetName()), x.AsVariable().Mul(m.AsVariable()))
}
