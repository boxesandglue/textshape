package ot

import (
	"encoding/binary"
)

// CharStringInterpreter interprets CFF Type 2 CharStrings to find subroutine usage.
type CharStringInterpreter struct {
	globalSubrs [][]byte
	localSubrs  [][]byte
	globalBias  int
	localBias   int

	// Closure tracking
	UsedGlobalSubrs map[int]bool
	UsedLocalSubrs  map[int]bool

	// Execution state
	stack     []int
	callStack []csCallFrame
	hintCount int
}

type csCallFrame struct {
	data []byte
	pos  int
}

// NewCharStringInterpreter creates a new interpreter for finding subroutine usage.
func NewCharStringInterpreter(globalSubrs, localSubrs [][]byte) *CharStringInterpreter {
	return &CharStringInterpreter{
		globalSubrs:     globalSubrs,
		localSubrs:      localSubrs,
		globalBias:      calcSubrBias(len(globalSubrs)),
		localBias:       calcSubrBias(len(localSubrs)),
		UsedGlobalSubrs: make(map[int]bool),
		UsedLocalSubrs:  make(map[int]bool),
		stack:           make([]int, 0, 48),
		callStack:       make([]csCallFrame, 0, 10),
	}
}

// FindUsedSubroutines executes a CharString to find all subroutine calls.
// This is a simplified interpreter that only tracks subr calls, not actual drawing.
func (i *CharStringInterpreter) FindUsedSubroutines(charstring []byte) error {
	i.stack = i.stack[:0]
	i.callStack = i.callStack[:0]
	i.hintCount = 0

	return i.execute(charstring)
}

func (i *CharStringInterpreter) execute(data []byte) error {
	pos := 0

	for pos < len(data) {
		b := data[pos]

		// Number operand
		if b >= 32 || b == 28 {
			val, consumed := decodeCSOperand(data[pos:])
			i.stack = append(i.stack, val)
			pos += consumed
			continue
		}

		// Operator
		op := int(b)
		pos++

		// Two-byte operator
		if b == 12 && pos < len(data) {
			op = 12<<8 | int(data[pos])
			pos++
		}

		switch op {
		case csCallsubr:
			// Call local subroutine
			if len(i.stack) > 0 {
				subrNum := i.stack[len(i.stack)-1] + i.localBias
				i.stack = i.stack[:len(i.stack)-1]

				if subrNum >= 0 && subrNum < len(i.localSubrs) {
					if !i.UsedLocalSubrs[subrNum] {
						i.UsedLocalSubrs[subrNum] = true
						// Recursively process subroutine
						i.callStack = append(i.callStack, csCallFrame{data: data, pos: pos})
						if err := i.execute(i.localSubrs[subrNum]); err != nil {
							return err
						}
						if len(i.callStack) > 0 {
							frame := i.callStack[len(i.callStack)-1]
							i.callStack = i.callStack[:len(i.callStack)-1]
							data = frame.data
							pos = frame.pos
						}
					}
				}
			}

		case csCallgsubr:
			// Call global subroutine
			if len(i.stack) > 0 {
				subrNum := i.stack[len(i.stack)-1] + i.globalBias
				i.stack = i.stack[:len(i.stack)-1]

				if subrNum >= 0 && subrNum < len(i.globalSubrs) {
					if !i.UsedGlobalSubrs[subrNum] {
						i.UsedGlobalSubrs[subrNum] = true
						// Recursively process subroutine
						i.callStack = append(i.callStack, csCallFrame{data: data, pos: pos})
						if err := i.execute(i.globalSubrs[subrNum]); err != nil {
							return err
						}
						if len(i.callStack) > 0 {
							frame := i.callStack[len(i.callStack)-1]
							i.callStack = i.callStack[:len(i.callStack)-1]
							data = frame.data
							pos = frame.pos
						}
					}
				}
			}

		case csReturn:
			// Return from subroutine
			return nil

		case csEndchar:
			// End of CharString
			return nil

		case csHstem, csVstem, csHstemhm, csVstemhm:
			// Hint operators - count stems and clear stack
			i.hintCount += len(i.stack) / 2
			i.stack = i.stack[:0]

		case csHintmask, csCntrmask:
			// Hint mask - implicit vstem if stack has data
			if len(i.stack) > 0 {
				i.hintCount += len(i.stack) / 2
				i.stack = i.stack[:0]
			}
			// Skip mask bytes
			maskBytes := (i.hintCount + 7) / 8
			pos += maskBytes

		case csRmoveto, csHmoveto, csVmoveto:
			// Movement operators clear stack
			i.stack = i.stack[:0]

		case csRlineto, csHlineto, csVlineto, csRrcurveto,
			csRcurveline, csRlinecurve, csVvcurveto, csHhcurveto,
			csVhcurveto, csHvcurveto:
			// Drawing operators clear stack
			i.stack = i.stack[:0]

		default:
			// Other operators - clear stack for safety
			if op >= 12<<8 {
				// Two-byte flex operators
				i.stack = i.stack[:0]
			}
		}
	}

	return nil
}

// decodeCSOperand decodes a CharString operand.
func decodeCSOperand(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}

	b0 := data[0]

	// 1-byte integer (32-246)
	if b0 >= 32 && b0 <= 246 {
		return int(b0) - 139, 1
	}

	// 2-byte positive integer (247-250)
	if b0 >= 247 && b0 <= 250 {
		if len(data) < 2 {
			return 0, 1
		}
		return (int(b0)-247)*256 + int(data[1]) + 108, 2
	}

	// 2-byte negative integer (251-254)
	if b0 >= 251 && b0 <= 254 {
		if len(data) < 2 {
			return 0, 1
		}
		return -(int(b0)-251)*256 - int(data[1]) - 108, 2
	}

	// 3-byte integer (operator 28)
	if b0 == 28 {
		if len(data) < 3 {
			return 0, 1
		}
		v := int(int16(binary.BigEndian.Uint16(data[1:3])))
		return v, 3
	}

	// Fixed-point number (255) - used in CharStrings only
	if b0 == 255 {
		if len(data) < 5 {
			return 0, 1
		}
		// 16.16 fixed point, return integer part
		v := int(int32(binary.BigEndian.Uint32(data[1:5])))
		return v >> 16, 5
	}

	return 0, 1
}

// csElementType represents the type of a parsed CharString element.
// HarfBuzz equivalent: parsed_cs_op_t in hb-subset-cff-common.hh
type csElementType int

const (
	// csElemBytes represents a sequence of raw bytes to copy (operands + operators)
	csElemBytes csElementType = iota
	// csElemCallSubr represents a callsubr operator (needs subroutine remapping)
	csElemCallSubr
	// csElemCallGSubr represents a callgsubr operator (needs subroutine remapping)
	csElemCallGSubr
)

// parsedCSElement represents a single parsed element from a CharString.
// This mirrors HarfBuzz's parsed_cs_op_t structure which stores pointers to original bytes.
//
// Key insight from HarfBuzz (hb-cff-interp-common.hh:483):
// - op_str_t stores ptr + length pointing to original bytes
// - For normal operators: copy original bytes directly
// - For callsubr/callgsubr: encode new biased number, then copy operator byte
type parsedCSElement struct {
	elemType csElementType
	// For csElemBytes: the raw bytes to copy (operands and/or operators)
	rawBytes []byte
	// For callsubr/callgsubr: the actual subroutine number (after adding bias)
	subrNum int
}

// parsedCharString holds a fully parsed CharString.
// HarfBuzz equivalent: parsed_cs_str_t in hb-subset-cff-common.hh
type parsedCharString struct {
	elements []parsedCSElement
	// Reference to original data for slicing
	original []byte
}

// parseCharString parses a CharString into a list of elements.
// This is the first phase of HarfBuzz's two-phase approach:
// 1. Parse completely into structured data
// 2. Serialize with remapped values
//
// Key insight from HarfBuzz (hb-subset-cff-common.hh):
// - Store pointers to original bytes, don't decode operand values
// - Only subroutine calls need special handling (to remap the subr number)
// - All other bytes are copied verbatim
func parseCharString(cs []byte, globalBias, localBias int) *parsedCharString {
	parsed := &parsedCharString{
		elements: make([]parsedCSElement, 0, 16),
		original: cs,
	}

	pos := 0
	opStart := 0 // Start of current operator + its operands (like HarfBuzz's opStart)
	hintCount := 0

	// Track operand positions to know where the subroutine number starts
	type operandPos struct {
		start int
		value int
	}
	operandStack := make([]operandPos, 0, 48)

	for pos < len(cs) {
		b := cs[pos]

		// Number operand - track position but don't add element yet
		if b >= 32 || b == 28 || b == 255 {
			operandStart := pos
			val, consumed := decodeCSOperand(cs[pos:])
			operandStack = append(operandStack, operandPos{start: operandStart, value: val})
			pos += consumed
			continue
		}

		// Operator
		op := int(b)
		pos++

		// Two-byte operator
		if b == 12 && pos < len(cs) {
			op = 12<<8 | int(cs[pos])
			pos++
		}

		switch op {
		case csCallsubr:
			if len(operandStack) > 0 {
				// Get the subroutine number
				subrOp := operandStack[len(operandStack)-1]
				operandStack = operandStack[:len(operandStack)-1]

				// Add raw bytes from opStart to just before the subr number operand
				if subrOp.start > opStart {
					parsed.elements = append(parsed.elements, parsedCSElement{
						elemType: csElemBytes,
						rawBytes: cs[opStart:subrOp.start],
					})
				}

				// Add callsubr element with actual subroutine number
				parsed.elements = append(parsed.elements, parsedCSElement{
					elemType: csElemCallSubr,
					subrNum:  subrOp.value + localBias,
				})

				opStart = pos
			}

		case csCallgsubr:
			if len(operandStack) > 0 {
				// Get the subroutine number
				subrOp := operandStack[len(operandStack)-1]
				operandStack = operandStack[:len(operandStack)-1]

				// Add raw bytes from opStart to just before the subr number operand
				if subrOp.start > opStart {
					parsed.elements = append(parsed.elements, parsedCSElement{
						elemType: csElemBytes,
						rawBytes: cs[opStart:subrOp.start],
					})
				}

				// Add callgsubr element with actual subroutine number
				parsed.elements = append(parsed.elements, parsedCSElement{
					elemType: csElemCallGSubr,
					subrNum:  subrOp.value + globalBias,
				})

				opStart = pos
			}

		case csHstem, csVstem, csHstemhm, csVstemhm:
			hintCount += len(operandStack) / 2
			operandStack = operandStack[:0]

		case csHintmask, csCntrmask:
			if len(operandStack) > 0 {
				hintCount += len(operandStack) / 2
				operandStack = operandStack[:0]
			}
			// Skip mask bytes
			maskBytes := (hintCount + 7) / 8
			pos += maskBytes

		default:
			operandStack = operandStack[:0]
		}
	}

	// Add any remaining bytes
	if pos > opStart {
		parsed.elements = append(parsed.elements, parsedCSElement{
			elemType: csElemBytes,
			rawBytes: cs[opStart:pos],
		})
	}

	return parsed
}

// serialize writes the parsed CharString back to bytes with remapped subroutine numbers.
// This is the second phase of HarfBuzz's approach.
//
// Key insight from HarfBuzz (hb-subset-cff-common.hh:1136-1150):
// - For callsubr/callgsubr: encode new biased number, then copy operator byte
// - For all other data: copy original bytes directly (no re-encoding!)
func (p *parsedCharString) serialize(globalMap, localMap map[int]int, newGlobalBias, newLocalBias int) []byte {
	// Estimate size: original length + some extra for potentially larger subr numbers
	result := make([]byte, 0, len(p.original)+16)

	for _, elem := range p.elements {
		switch elem.elemType {
		case csElemBytes:
			// Copy original bytes verbatim - this preserves fixed-point precision!
			result = append(result, elem.rawBytes...)

		case csElemCallSubr:
			// Remap local subroutine
			if newNum, ok := localMap[elem.subrNum]; ok {
				biasedNum := newNum - newLocalBias
				result = append(result, encodeCSInt(biasedNum)...)
			} else {
				// Subroutine not in map - shouldn't happen if closure was computed correctly
				biasedNum := elem.subrNum - newLocalBias
				result = append(result, encodeCSInt(biasedNum)...)
			}
			result = append(result, byte(csCallsubr))

		case csElemCallGSubr:
			// Remap global subroutine
			if newNum, ok := globalMap[elem.subrNum]; ok {
				biasedNum := newNum - newGlobalBias
				result = append(result, encodeCSInt(biasedNum)...)
			} else {
				// Subroutine not in map - shouldn't happen if closure was computed correctly
				biasedNum := elem.subrNum - newGlobalBias
				result = append(result, encodeCSInt(biasedNum)...)
			}
			result = append(result, byte(csCallgsubr))
		}
	}

	return result
}

// RemapCharString rewrites a CharString with remapped subroutine numbers.
// This is the HarfBuzz-style implementation using two phases:
// 1. Parse the CharString completely, storing pointers to original bytes
// 2. Serialize: copy original bytes for everything except subroutine calls
//
// Key insight from HarfBuzz (hb-subset-cff-common.hh):
// - Original bytes are preserved for operands (including fixed-point numbers)
// - Only subroutine numbers need to be re-encoded with new biased values
func RemapCharString(cs []byte, globalMap, localMap map[int]int,
	oldGlobalBias, oldLocalBias, newGlobalBias, newLocalBias int) []byte {

	// Phase 1: Parse completely
	parsed := parseCharString(cs, oldGlobalBias, oldLocalBias)

	// Check if any remapping is needed
	hasSubrCalls := false
	for _, elem := range parsed.elements {
		if elem.elemType == csElemCallSubr || elem.elemType == csElemCallGSubr {
			hasSubrCalls = true
			break
		}
	}

	// If no subroutine calls, return original unchanged
	if !hasSubrCalls {
		return cs
	}

	// Phase 2: Serialize with remapped values
	return parsed.serialize(globalMap, localMap, newGlobalBias, newLocalBias)
}
