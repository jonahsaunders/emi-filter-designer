package engine

// Modified Nodal Analysis (MNA) small-signal AC solver.
//
// Supports R, C, L (with branch currents), mutual coupling K between
// inductors, independent current sources and independent voltage sources.
// Sources are "tagged" so that several excitations (e.g. DM noise and CM
// noise) can be solved with one LU factorisation (multiple right-hand sides).

import (
	"fmt"
	"math"
	"math/cmplx"
)

type primKind int

const (
	pR primKind = iota
	pC
	pL
	pI
	pV
	pK
)

type prim struct {
	kind   primKind
	name   string
	a, b   int
	val    float64
	br     int // branch index (L, V)
	l1, l2 int // prim indices of coupled inductors (K)
	tag    string
}

// Netlist is a flat list of primitive elements.
type Netlist struct {
	nodeIdx   map[string]int
	NodeNames []string
	prims     []prim
	nBranch   int
	lIndex    map[string]int
	tags      map[string]bool
}

func NewNetlist() *Netlist {
	return &Netlist{nodeIdx: map[string]int{}, lIndex: map[string]int{}, tags: map[string]bool{}}
}

func isGround(n string) bool { return n == "0" || n == "GND" || n == "PE" }

// Node returns the index of a node (-1 for ground), creating it if required.
func (n *Netlist) Node(name string) int {
	if isGround(name) {
		return -1
	}
	if i, ok := n.nodeIdx[name]; ok {
		return i
	}
	i := len(n.NodeNames)
	n.nodeIdx[name] = i
	n.NodeNames = append(n.NodeNames, name)
	return i
}

// HasNode reports whether a node exists.
func (n *Netlist) HasNode(name string) bool {
	if isGround(name) {
		return true
	}
	_, ok := n.nodeIdx[name]
	return ok
}

func (n *Netlist) R(name, a, b string, r float64) {
	if r <= 0 {
		r = 1e-6
	}
	n.prims = append(n.prims, prim{kind: pR, name: name, a: n.Node(a), b: n.Node(b), val: r})
}

func (n *Netlist) C(name, a, b string, c float64) {
	if c <= 0 {
		return
	}
	n.prims = append(n.prims, prim{kind: pC, name: name, a: n.Node(a), b: n.Node(b), val: c})
}

// L adds an inductor with its own branch current (needed for coupling).
func (n *Netlist) L(name, a, b string, l float64) {
	if l < 0 {
		l = 0
	}
	n.lIndex[name] = len(n.prims)
	n.prims = append(n.prims, prim{kind: pL, name: name, a: n.Node(a), b: n.Node(b), val: l, br: n.nBranch})
	n.nBranch++
}

// K couples two previously added inductors with coupling coefficient k.
func (n *Netlist) K(name, l1, l2 string, k float64) error {
	i1, ok1 := n.lIndex[l1]
	i2, ok2 := n.lIndex[l2]
	if !ok1 || !ok2 {
		return fmt.Errorf("coupling %s: unknown inductor", name)
	}
	if k > 0.99999 {
		k = 0.99999
	}
	n.prims = append(n.prims, prim{kind: pK, name: name, l1: i1, l2: i2, val: k})
	return nil
}

// I adds a current source; positive current flows from node a, through the
// source, into node b (i.e. it is drawn out of a and pushed into b).
func (n *Netlist) I(name, a, b, tag string) {
	n.tags[tag] = true
	n.prims = append(n.prims, prim{kind: pI, name: name, a: n.Node(a), b: n.Node(b), tag: tag})
}

// V adds a voltage source with V(a) - V(b) = amplitude(tag).
func (n *Netlist) V(name, a, b, tag string) {
	n.tags[tag] = true
	n.prims = append(n.prims, prim{kind: pV, name: name, a: n.Node(a), b: n.Node(b), tag: tag, br: n.nBranch})
	n.nBranch++
}

// Solution holds node voltages for one excitation.
type Solution struct {
	nl *Netlist
	x  []complex128
}

func (s Solution) V(node string) complex128 {
	if isGround(node) {
		return 0
	}
	i, ok := s.nl.nodeIdx[node]
	if !ok {
		return 0
	}
	return s.x[i]
}

// Solve solves the circuit at frequency f for each excitation.  Each
// excitation maps a source tag to its complex amplitude; sources whose tag is
// absent are zero.
func (n *Netlist) Solve(f float64, exc []map[string]complex128) ([]Solution, error) {
	nn := len(n.NodeNames)
	N := nn + n.nBranch
	m := len(exc)
	A := make([][]complex128, N)
	for i := range A {
		A[i] = make([]complex128, N+m)
	}
	w := 2 * math.Pi * f
	jw := complex(0, w)
	stampY := func(a, b int, y complex128) {
		if a >= 0 {
			A[a][a] += y
		}
		if b >= 0 {
			A[b][b] += y
		}
		if a >= 0 && b >= 0 {
			A[a][b] -= y
			A[b][a] -= y
		}
	}
	// gmin for robustness against floating nodes
	for i := 0; i < nn; i++ {
		A[i][i] += 1e-12
	}
	for _, p := range n.prims {
		switch p.kind {
		case pR:
			stampY(p.a, p.b, complex(1/p.val, 0))
		case pC:
			stampY(p.a, p.b, jw*complex(p.val, 0))
		case pL:
			k := nn + p.br
			if p.a >= 0 {
				A[p.a][k] += 1
				A[k][p.a] += 1
			}
			if p.b >= 0 {
				A[p.b][k] -= 1
				A[k][p.b] -= 1
			}
			A[k][k] -= jw * complex(p.val, 0)
		case pK:
			L1 := n.prims[p.l1]
			L2 := n.prims[p.l2]
			M := p.val * math.Sqrt(L1.val*L2.val)
			k1 := nn + L1.br
			k2 := nn + L2.br
			A[k1][k2] -= jw * complex(M, 0)
			A[k2][k1] -= jw * complex(M, 0)
		case pV:
			k := nn + p.br
			if p.a >= 0 {
				A[p.a][k] += 1
				A[k][p.a] += 1
			}
			if p.b >= 0 {
				A[p.b][k] -= 1
				A[k][p.b] -= 1
			}
			for j, e := range exc {
				A[k][N+j] += e[p.tag]
			}
		case pI:
			for j, e := range exc {
				v := e[p.tag]
				if p.a >= 0 {
					A[p.a][N+j] -= v
				}
				if p.b >= 0 {
					A[p.b][N+j] += v
				}
			}
		}
	}
	if err := gaussSolve(A, N, m); err != nil {
		return nil, err
	}
	out := make([]Solution, m)
	for j := 0; j < m; j++ {
		x := make([]complex128, N)
		for i := 0; i < N; i++ {
			x[i] = A[i][N+j]
		}
		out[j] = Solution{nl: n, x: x}
	}
	return out, nil
}

// gaussSolve performs in-place Gauss-Jordan elimination with partial pivoting
// on an augmented matrix with m right-hand-side columns.
func gaussSolve(A [][]complex128, N, m int) error {
	for c := 0; c < N; c++ {
		piv := c
		best := cmplx.Abs(A[c][c])
		for r := c + 1; r < N; r++ {
			if v := cmplx.Abs(A[r][c]); v > best {
				best, piv = v, r
			}
		}
		if best == 0 || math.IsNaN(best) {
			return fmt.Errorf("singular circuit matrix (column %d)", c)
		}
		A[c], A[piv] = A[piv], A[c]
		inv := 1 / A[c][c]
		rowc := A[c]
		for r := c + 1; r < N; r++ {
			f := A[r][c] * inv
			if f == 0 {
				continue
			}
			rowr := A[r]
			for k := c; k < N+m; k++ {
				rowr[k] -= f * rowc[k]
			}
		}
	}
	// back substitution
	for j := 0; j < m; j++ {
		for r := N - 1; r >= 0; r-- {
			s := A[r][N+j]
			for k := r + 1; k < N; k++ {
				s -= A[r][k] * A[k][N+j]
			}
			A[r][N+j] = s / A[r][r]
		}
	}
	return nil
}
