// Package learn trains and evaluates the routing artifact. Everything is pure
// Go without third-party numeric libraries: the data sets are a few hundred
// examples, so plain full-batch optimisation is fast enough (well under a
// second) and keeps the binary free of cgo.
package learn

import (
	"maps"
	"math"
	"slices"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/service"
)

// SoftmaxModel is a multinomial logistic regression (arXiv:2608.00106 uses
// word + character n-gram logistic regression as the router's classifier).
type SoftmaxModel struct {
	W [][]float64
	B []float64
}

// TrainSoftmax fits W and b by full-batch gradient descent with Adam. Every
// optimiser parameter comes from [training] in the settings.
func TrainSoftmax(xs []service.Vector, ys []int, classes, dim int, opt model.TrainingSettings) SoftmaxModel {
	m := SoftmaxModel{W: make([][]float64, classes), B: make([]float64, classes)}
	mw, vw := make([][]float64, classes), make([][]float64, classes)
	for c := range classes {
		m.W[c] = make([]float64, dim)
		mw[c], vw[c] = make([]float64, dim), make([]float64, dim)
	}
	mb, vb := make([]float64, classes), make([]float64, classes)
	beta1, beta2, eps := opt.AdamBeta1, opt.AdamBeta2, opt.AdamEps
	n := float64(max(len(xs), 1))

	for epoch := range opt.Epochs {
		gw := make([][]float64, classes)
		for c := range classes {
			gw[c] = make([]float64, dim)
		}
		gb := make([]float64, classes)
		for i, x := range xs {
			p := service.Softmax(m.logits(x), 1)
			for c := range classes {
				d := p[c]
				if c == ys[i] {
					d--
				}
				gb[c] += d / n
				for k, idx := range x.Idx {
					gw[c][idx] += d * float64(x.Val[k]) / n
				}
			}
		}
		t := float64(epoch + 1)
		for c := range classes {
			for j := range dim {
				g := gw[c][j] + opt.L2*m.W[c][j]
				if g == 0 && m.W[c][j] == 0 {
					continue
				}
				mw[c][j] = beta1*mw[c][j] + (1-beta1)*g
				vw[c][j] = beta2*vw[c][j] + (1-beta2)*g*g
				m.W[c][j] -= opt.LearningRate * (mw[c][j] / (1 - math.Pow(beta1, t))) /
					(math.Sqrt(vw[c][j]/(1-math.Pow(beta2, t))) + eps)
			}
			mb[c] = beta1*mb[c] + (1-beta1)*gb[c]
			vb[c] = beta2*vb[c] + (1-beta2)*gb[c]*gb[c]
			m.B[c] -= opt.LearningRate * (mb[c] / (1 - math.Pow(beta1, t))) /
				(math.Sqrt(vb[c]/(1-math.Pow(beta2, t))) + eps)
		}
	}
	return m
}

func (m SoftmaxModel) logits(x service.Vector) []float64 {
	out := make([]float64, len(m.W))
	for c := range m.W {
		out[c] = service.Dot(x, m.W[c]) + m.B[c]
	}
	return out
}

// Logits returns W·x + b.
func (m SoftmaxModel) Logits(x service.Vector) []float64 { return m.logits(x) }

// probabilityFloor keeps log(0) out of the likelihood (numerical, not tunable).
const probabilityFloor = 1e-12

// FitTemperature picks T minimising the negative log-likelihood of held-out
// logits (temperature scaling; arXiv:2608.00106 calibrates the same way).
func FitTemperature(logits [][]float64, ys []int, grid model.Grid) float64 {
	values := grid.Values()
	best, bestNLL := values[0], math.Inf(1)
	for _, t := range values {
		nll := 0.0
		for i, z := range logits {
			nll -= math.Log(max(service.Softmax(z, t)[ys[i]], probabilityFloor))
		}
		if nll < bestNLL {
			best, bestNLL = t, nll
		}
	}
	return best
}

// Folds returns k disjoint index sets, stratified by label so every fold
// sees every class (deterministic: no randomness, reproducible artifacts).
func Folds(ys []int, k int) [][]int {
	folds := make([][]int, k)
	byLabel := map[int][]int{}
	for i, y := range ys {
		byLabel[y] = append(byLabel[y], i)
	}
	labels := slices.Sorted(maps.Keys(byLabel))
	next := 0
	for _, l := range labels {
		for _, i := range byLabel[l] {
			folds[next%k] = append(folds[next%k], i)
			next++
		}
	}
	return folds
}
