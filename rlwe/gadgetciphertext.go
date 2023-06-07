package rlwe

import (
	"bytes"
	"io"

	"github.com/google/go-cmp/cmp"
	"github.com/tuneinsight/lattigo/v4/ring"
	"github.com/tuneinsight/lattigo/v4/rlwe/ringqp"
	"github.com/tuneinsight/lattigo/v4/utils/structs"
)

// GadgetCiphertext is a struct for storing an encrypted
// plaintext times the gadget power matrix.
type GadgetCiphertext struct {
	Value structs.Matrix[OperandQP]
}

// NewGadgetCiphertext returns a new Ciphertext key with pre-allocated zero-value.
// Ciphertext is always in the NTT domain.
func NewGadgetCiphertext(params ParametersInterface, levelQ, levelP, decompRNS, decompBIT int) *GadgetCiphertext {

	m := make([][]OperandQP, decompRNS)
	for i := 0; i < decompRNS; i++ {
		v := make([]OperandQP, decompBIT)

		for j := range v {
			v[j] = *NewOperandQP(params, 1, levelQ, levelP)
			v[j].IsNTT = true
			v[j].IsMontgomery = true
		}

		m[i] = v
	}

	return &GadgetCiphertext{Value: m}
}

// LevelQ returns the level of the modulus Q of the target Ciphertext.
func (ct GadgetCiphertext) LevelQ() int {
	return ct.Value[0][0].LevelQ()
}

// LevelP returns the level of the modulus P of the target Ciphertext.
func (ct GadgetCiphertext) LevelP() int {
	return ct.Value[0][0].LevelP()
}

// Equal checks two Ciphertexts for equality.
func (ct *GadgetCiphertext) Equal(other *GadgetCiphertext) bool {
	return cmp.Equal(ct.Value, other.Value)
}

// CopyNew creates a deep copy of the receiver Ciphertext and returns it.
func (ct *GadgetCiphertext) CopyNew() (ctCopy *GadgetCiphertext) {
	if ct == nil || len(ct.Value) == 0 {
		return nil
	}
	v := make([][]OperandQP, len(ct.Value))
	for i := range ct.Value {
		v[i] = make([]OperandQP, len(ct.Value[0]))
		for j, el := range ct.Value[i] {
			v[i][j] = *el.CopyNew()
		}
	}
	return &GadgetCiphertext{Value: v}
}

// MarshalBinary encodes the object into a binary form on a newly allocated slice of bytes.
func (ct *GadgetCiphertext) MarshalBinary() (data []byte, err error) {
	buf := bytes.NewBuffer([]byte{})
	_, err = ct.WriteTo(buf)
	return buf.Bytes(), err
}

// UnmarshalBinary decodes a slice of bytes generated by
// MarshalBinary or WriteTo on the object.
func (ct *GadgetCiphertext) UnmarshalBinary(p []byte) (err error) {
	_, err = ct.ReadFrom(bytes.NewBuffer(p))
	return
}

// WriteTo writes the object on an io.Writer.
// To ensure optimal efficiency and minimal allocations, the user is encouraged
// to provide a struct implementing the interface buffer.Writer, which defines
// a subset of the method of the bufio.Writer.
// If w is not compliant to the buffer.Writer interface, it will be wrapped in
// a new bufio.Writer.
// For additional information, see lattigo/utils/buffer/writer.go.
func (ct *GadgetCiphertext) WriteTo(w io.Writer) (n int64, err error) {
	return ct.Value.WriteTo(w)
}

// ReadFrom reads on the object from an io.Writer.
// To ensure optimal efficiency and minimal allocations, the user is encouraged
// to provide a struct implementing the interface buffer.Reader, which defines
// a subset of the method of the bufio.Reader.
// If r is not compliant to the buffer.Reader interface, it will be wrapped in
// a new bufio.Reader.
// For additional information, see lattigo/utils/buffer/reader.go.
func (ct *GadgetCiphertext) ReadFrom(r io.Reader) (n int64, err error) {
	return ct.Value.ReadFrom(r)
}

// BinarySize returns the size in bytes of the object
// when encoded using Encode.
func (ct *GadgetCiphertext) BinarySize() (dataLen int) {
	return ct.Value.BinarySize()
}

// Encode encodes the object into a binary form on a preallocated slice of bytes
// and returns the number of bytes written.
func (ct *GadgetCiphertext) Encode(p []byte) (n int, err error) {
	return ct.Value.Encode(p)
}

// Decode decodes a slice of bytes generated by Encode
// on the object and returns the number of bytes read.
func (ct *GadgetCiphertext) Decode(p []byte) (n int, err error) {
	return ct.Value.Decode(p)
}

// AddPolyTimesGadgetVectorToGadgetCiphertext takes a plaintext polynomial and a list of Ciphertexts and adds the
// plaintext times the RNS and BIT decomposition to the i-th element of the i-th Ciphertexts. This method panics if
// len(cts) > 2.
func AddPolyTimesGadgetVectorToGadgetCiphertext(pt *ring.Poly, cts []GadgetCiphertext, ringQP ringqp.Ring, logbase2 int, buff *ring.Poly) {

	levelQ := cts[0].LevelQ()
	levelP := cts[0].LevelP()

	ringQ := ringQP.RingQ.AtLevel(levelQ)

	if len(cts) > 2 {
		panic("cannot AddPolyTimesGadgetVectorToGadgetCiphertext: len(cts) should be <= 2")
	}

	if levelP != -1 {
		ringQ.MulScalarBigint(pt, ringQP.RingP.AtLevel(levelP).Modulus(), buff) // P * pt
	} else {
		levelP = 0
		if pt != buff {
			ring.CopyLvl(levelQ, pt, buff) // 1 * pt
		}
	}

	RNSDecomp := len(cts[0].Value)
	BITDecomp := len(cts[0].Value[0])
	N := ringQ.N()

	var index int
	for j := 0; j < BITDecomp; j++ {
		for i := 0; i < RNSDecomp; i++ {

			// e + (m * P * w^2j) * (q_star * q_tild) mod QP
			//
			// q_prod = prod(q[i*#Pi+j])
			// q_star = Q/qprod
			// q_tild = q_star^-1 mod q_prod
			//
			// Therefore : (pt * P * w^2j) * (q_star * q_tild) = pt*P*w^2j mod q[i*#Pi+j], else 0
			for k := 0; k < levelP+1; k++ {

				index = i*(levelP+1) + k

				// Handle cases where #pj does not divide #qi
				if index >= levelQ+1 {
					break
				}

				qi := ringQ.SubRings[index].Modulus
				p0tmp := buff.Coeffs[index]

				for u, ct := range cts {
					p1tmp := ct.Value[i][j].Value[u].Q.Coeffs[index]
					for w := 0; w < N; w++ {
						p1tmp[w] = ring.CRed(p1tmp[w]+p0tmp[w], qi)
					}
				}

			}
		}

		// w^2j
		ringQ.MulScalar(buff, 1<<logbase2, buff)
	}
}

// GadgetPlaintext stores a plaintext value times the gadget vector.
type GadgetPlaintext struct {
	Value structs.Vector[ring.Poly]
}

// NewGadgetPlaintext creates a new gadget plaintext from value, which can be either uint64, int64 or *ring.Poly.
// Plaintext is returned in the NTT and Mongtomery domain.
func NewGadgetPlaintext(params Parameters, value interface{}, levelQ, levelP, logBase2, decompBIT int) (pt *GadgetPlaintext) {

	ringQ := params.RingQP().RingQ.AtLevel(levelQ)

	pt = new(GadgetPlaintext)
	pt.Value = make([]ring.Poly, decompBIT)

	switch el := value.(type) {
	case uint64:
		pt.Value[0] = *ringQ.NewPoly()
		for i := 0; i < levelQ+1; i++ {
			pt.Value[0].Coeffs[i][0] = el
		}
	case int64:
		pt.Value[0] = *ringQ.NewPoly()
		if el < 0 {
			for i := 0; i < levelQ+1; i++ {
				pt.Value[0].Coeffs[i][0] = ringQ.SubRings[i].Modulus - uint64(-el)
			}
		} else {
			for i := 0; i < levelQ+1; i++ {
				pt.Value[0].Coeffs[i][0] = uint64(el)
			}
		}
	case *ring.Poly:
		pt.Value[0] = *el.CopyNew()
	default:
		panic("cannot NewGadgetPlaintext: unsupported type, must be wither uint64 or *ring.Poly")
	}

	if levelP > -1 {
		ringQ.MulScalarBigint(&pt.Value[0], params.RingP().AtLevel(levelP).Modulus(), &pt.Value[0])
	}

	ringQ.NTT(&pt.Value[0], &pt.Value[0])
	ringQ.MForm(&pt.Value[0], &pt.Value[0])

	for i := 1; i < len(pt.Value); i++ {

		pt.Value[i] = *pt.Value[0].CopyNew()

		for j := 0; j < i; j++ {
			ringQ.MulScalar(&pt.Value[i], 1<<logBase2, &pt.Value[i])
		}
	}

	return
}
