package ot

import (
	"encoding/binary"
	"math"
)

// cffDrawInterpreter interprets CFF Type 2 CharStrings to produce glyph outlines.
// HarfBuzz equivalent: cs_interpreter_t + cff1_cs_opset_t + path_procs_t
// in hb-cff-interp-cs-common.hh and hb-cff1-interp-cs.hh
type cffDrawInterpreter struct {
	segments []Segment

	// Argument stack. HarfBuzz uses double (number_t), we use float64.
	// HarfBuzz equivalent: arg_stack_t in hb-cff-interp-common.hh:428
	stack    []float64
	argStart int // offset to skip width argument; HarfBuzz: arg_start in hb-cff1-interp-cs.hh:75

	// Current point (absolute coordinates).
	// HarfBuzz equivalent: point_t pt in cs_interp_env_t (hb-cff-interp-cs-common.hh:209)
	x, y float64

	// Width handling.
	// HarfBuzz equivalent: processed_width, has_width in hb-cff1-interp-cs.hh:73-74
	processedWidth bool

	// Hint tracking for hintmask/cntrmask byte skipping.
	// HarfBuzz equivalent: hstem_count, vstem_count in hb-cff-interp-cs-common.hh:201-202
	hstemCount int
	vstemCount int

	// Subroutine data.
	globalSubrs [][]byte
	localSubrs  [][]byte
	globalBias  int
	localBias   int

	// Call stack for subroutine calls. Max depth 10.
	// HarfBuzz equivalent: callStack in hb-cff-interp-cs-common.hh:204
	callDepth int

	// Operation counter to prevent infinite loops.
	// HarfBuzz equivalent: max_ops in cs_interpreter_t::interpret (hb-cff-interp-cs-common.hh:884)
	opsCount int

	err bool
}

const cffMaxOps = 200000   // HarfBuzz: HB_CFF_MAX_OPS
const cffMaxCallDepth = 10 // HarfBuzz: kMaxCallLimit

// cffGlyphOutline extracts the outline for a CFF glyph.
// HarfBuzz equivalent: OT::cff1::accelerator_t::get_path in hb-ot-cff1-table.cc:444-564
func cffGlyphOutline(cff *CFF, gid GlyphID) (GlyphOutline, bool) {
	if int(gid) >= len(cff.CharStrings) {
		return GlyphOutline{}, false
	}
	cs := cff.CharStrings[gid]
	if len(cs) == 0 {
		return GlyphOutline{}, false
	}

	interp := cffDrawInterpreter{
		stack:       make([]float64, 0, 48),
		globalSubrs: cff.GlobalSubrs,
		localSubrs:  cff.LocalSubrs,
		globalBias:  calcSubrBias(len(cff.GlobalSubrs)),
		localBias:   calcSubrBias(len(cff.LocalSubrs)),
	}

	interp.execute(cs)

	if interp.err || len(interp.segments) == 0 {
		return GlyphOutline{}, false
	}
	return GlyphOutline{Segments: interp.segments}, true
}

// argCount returns the number of arguments on the stack (accounting for arg_start).
// HarfBuzz equivalent: argStack.get_count() adjusted by arg_start
func (di *cffDrawInterpreter) argCount() int {
	return len(di.stack) - di.argStart
}

// evalArg returns the argument at index i (relative to argStart).
// HarfBuzz equivalent: env.eval_arg(i) which accesses argStack[arg_start + i]
func (di *cffDrawInterpreter) evalArg(i int) float64 {
	return di.stack[di.argStart+i]
}

// popArg pops the top argument from the stack.
// HarfBuzz equivalent: env.pop_arg()
func (di *cffDrawInterpreter) popArg() float64 {
	if len(di.stack) == 0 {
		di.err = true
		return 0
	}
	v := di.stack[len(di.stack)-1]
	di.stack = di.stack[:len(di.stack)-1]
	return v
}

// clearArgs clears the argument stack and resets argStart.
// HarfBuzz equivalent: cff1_cs_interp_env_t::clear_args in hb-cff1-interp-cs.hh:65-69
func (di *cffDrawInterpreter) clearArgs() {
	di.argStart = 0
	di.stack = di.stack[:0]
}

// checkWidth detects and skips the optional width argument.
// HarfBuzz equivalent: cff1_cs_opset_t::check_width in hb-cff1-interp-cs.hh:111-139
func (di *cffDrawInterpreter) checkWidth(op int) {
	if di.processedWidth {
		return
	}
	hasWidth := false
	switch op {
	case csEndchar, csHstem, csHstemhm, csVstem, csVstemhm, csHintmask, csCntrmask:
		hasWidth = (di.argCount() & 1) != 0
	case csHmoveto, csVmoveto:
		hasWidth = di.argCount() > 1
	case csRmoveto:
		hasWidth = di.argCount() > 2
	default:
		return
	}
	if hasWidth && len(di.stack) > 0 {
		di.argStart = 1
	}
	di.processedWidth = true
}

// execute interprets a CharString byte stream.
// HarfBuzz equivalent: cs_interpreter_t::interpret in hb-cff-interp-cs-common.hh:880-897
func (di *cffDrawInterpreter) execute(data []byte) {
	pos := 0
	for pos < len(data) {
		if di.err {
			return
		}
		di.opsCount++
		if di.opsCount > cffMaxOps {
			di.err = true
			return
		}

		b := data[pos]

		// Number operand.
		// HarfBuzz equivalent: number encoding in hb-cff-interp-common.hh:563-633
		if b >= 32 || b == 28 || b == 255 {
			val, consumed := decodeCSOperandFloat(data[pos:])
			di.stack = append(di.stack, val)
			pos += consumed
			continue
		}

		// Operator.
		op := int(b)
		pos++

		// Two-byte operator (escape prefix 12).
		if b == 12 && pos < len(data) {
			op = 12<<8 | int(data[pos])
			pos++
		}

		switch op {

		// --- Hint operators ---
		// HarfBuzz: cs_opset_t::process_op cases for OpCode_hstem etc.
		// hb-cff-interp-cs-common.hh:267-281
		case csHstem, csHstemhm:
			di.checkWidth(op)
			di.hstemCount += di.argCount() / 2
			di.clearArgs()

		case csVstem, csVstemhm:
			di.checkWidth(op)
			di.vstemCount += di.argCount() / 2
			di.clearArgs()

		case csHintmask, csCntrmask:
			di.checkWidth(op)
			// Implicit vstem if args remain.
			if di.argCount() > 0 {
				di.vstemCount += di.argCount() / 2
			}
			di.clearArgs()
			// Skip hint mask bytes.
			// HarfBuzz: determine_hintmask_size in hb-cff-interp-cs-common.hh:174-182
			maskBytes := (di.hstemCount + di.vstemCount + 7) / 8
			pos += maskBytes

		// --- Move operators ---
		// HarfBuzz: path_procs_t::rmoveto/hmoveto/vmoveto in hb-cff-interp-cs-common.hh:454-475
		case csRmoveto:
			di.checkWidth(op)
			dy := di.popArg()
			dx := di.popArg()
			di.x += dx
			di.y += dy
			di.emitMoveTo()
			di.clearArgs()

		case csHmoveto:
			di.checkWidth(op)
			dx := di.popArg()
			di.x += dx
			di.emitMoveTo()
			di.clearArgs()

		case csVmoveto:
			di.checkWidth(op)
			dy := di.popArg()
			di.y += dy
			di.emitMoveTo()
			di.clearArgs()

		// --- Line operators ---
		// HarfBuzz: path_procs_t::rlineto in hb-cff-interp-cs-common.hh:477-485
		case csRlineto:
			for i := 0; i+2 <= di.argCount(); i += 2 {
				di.x += di.evalArg(i)
				di.y += di.evalArg(i + 1)
				di.emitLineTo()
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::hlineto in hb-cff-interp-cs-common.hh:487-505
		case csHlineto:
			i := 0
			for i+2 <= di.argCount() {
				di.x += di.evalArg(i)
				di.emitLineTo()
				di.y += di.evalArg(i + 1)
				di.emitLineTo()
				i += 2
			}
			if i < di.argCount() {
				di.x += di.evalArg(i)
				di.emitLineTo()
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::vlineto in hb-cff-interp-cs-common.hh:507-525
		case csVlineto:
			i := 0
			for i+2 <= di.argCount() {
				di.y += di.evalArg(i)
				di.emitLineTo()
				di.x += di.evalArg(i + 1)
				di.emitLineTo()
				i += 2
			}
			if i < di.argCount() {
				di.y += di.evalArg(i)
				di.emitLineTo()
			}
			di.clearArgs()

		// --- Curve operators ---
		// HarfBuzz: path_procs_t::rrcurveto in hb-cff-interp-cs-common.hh:527-539
		case csRrcurveto:
			for i := 0; i+6 <= di.argCount(); i += 6 {
				di.emitRRCurveTo(
					di.evalArg(i), di.evalArg(i+1),
					di.evalArg(i+2), di.evalArg(i+3),
					di.evalArg(i+4), di.evalArg(i+5),
				)
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::rcurveline in hb-cff-interp-cs-common.hh:541-563
		case csRcurveline:
			ac := di.argCount()
			if ac < 8 {
				di.clearArgs()
				break
			}
			i := 0
			curveLimit := ac - 2
			for i+6 <= curveLimit {
				di.emitRRCurveTo(
					di.evalArg(i), di.evalArg(i+1),
					di.evalArg(i+2), di.evalArg(i+3),
					di.evalArg(i+4), di.evalArg(i+5),
				)
				i += 6
			}
			di.x += di.evalArg(i)
			di.y += di.evalArg(i + 1)
			di.emitLineTo()
			di.clearArgs()

		// HarfBuzz: path_procs_t::rlinecurve in hb-cff-interp-cs-common.hh:565-587
		case csRlinecurve:
			ac := di.argCount()
			if ac < 8 {
				di.clearArgs()
				break
			}
			i := 0
			lineLimit := ac - 6
			for i+2 <= lineLimit {
				di.x += di.evalArg(i)
				di.y += di.evalArg(i + 1)
				di.emitLineTo()
				i += 2
			}
			di.emitRRCurveTo(
				di.evalArg(i), di.evalArg(i+1),
				di.evalArg(i+2), di.evalArg(i+3),
				di.evalArg(i+4), di.evalArg(i+5),
			)
			di.clearArgs()

		// HarfBuzz: path_procs_t::vvcurveto in hb-cff-interp-cs-common.hh:589-605
		case csVvcurveto:
			i := 0
			dx1 := float64(0)
			if (di.argCount() & 1) != 0 {
				dx1 = di.evalArg(i)
				i++
			}
			for i+4 <= di.argCount() {
				pt1x := di.x + dx1
				pt1y := di.y + di.evalArg(i)
				pt2x := pt1x + di.evalArg(i+1)
				pt2y := pt1y + di.evalArg(i+2)
				pt3x := pt2x
				pt3y := pt2y + di.evalArg(i+3)
				di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)
				dx1 = 0
				i += 4
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::hhcurveto in hb-cff-interp-cs-common.hh:607-623
		case csHhcurveto:
			i := 0
			dy1 := float64(0)
			if (di.argCount() & 1) != 0 {
				dy1 = di.evalArg(i)
				i++
			}
			for i+4 <= di.argCount() {
				pt1x := di.x + di.evalArg(i)
				pt1y := di.y + dy1
				pt2x := pt1x + di.evalArg(i+1)
				pt2y := pt1y + di.evalArg(i+2)
				pt3x := pt2x + di.evalArg(i+3)
				pt3y := pt2y
				di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)
				dy1 = 0
				i += 4
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::vhcurveto in hb-cff-interp-cs-common.hh:625-684
		case csVhcurveto:
			di.processAlternatingCurves(true)
			di.clearArgs()

		// HarfBuzz: path_procs_t::hvcurveto in hb-cff-interp-cs-common.hh:686-744
		case csHvcurveto:
			di.processAlternatingCurves(false)
			di.clearArgs()

		// --- Flex operators ---
		// HarfBuzz: path_procs_t::hflex in hb-cff-interp-cs-common.hh:757-779
		case csHflex:
			if di.argCount() == 7 {
				startY := di.y
				pt1x := di.x + di.evalArg(0)
				pt1y := di.y
				pt2x := pt1x + di.evalArg(1)
				pt2y := pt1y + di.evalArg(2)
				pt3x := pt2x + di.evalArg(3)
				pt3y := pt2y
				di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

				pt4x := di.x + di.evalArg(4)
				pt4y := di.y
				pt5x := pt4x + di.evalArg(5)
				pt5y := startY
				pt6x := pt5x + di.evalArg(6)
				pt6y := pt5y
				di.emitCubicTo(pt4x, pt4y, pt5x, pt5y, pt6x, pt6y)
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::flex in hb-cff-interp-cs-common.hh:781-802
		case csFlex:
			if di.argCount() == 13 {
				pt1x := di.x + di.evalArg(0)
				pt1y := di.y + di.evalArg(1)
				pt2x := pt1x + di.evalArg(2)
				pt2y := pt1y + di.evalArg(3)
				pt3x := pt2x + di.evalArg(4)
				pt3y := pt2y + di.evalArg(5)
				di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

				pt4x := di.x + di.evalArg(6)
				pt4y := di.y + di.evalArg(7)
				pt5x := pt4x + di.evalArg(8)
				pt5y := pt4y + di.evalArg(9)
				pt6x := pt5x + di.evalArg(10)
				pt6y := pt5y + di.evalArg(11)
				di.emitCubicTo(pt4x, pt4y, pt5x, pt5y, pt6x, pt6y)
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::hflex1 in hb-cff-interp-cs-common.hh:804-826
		case csHflex1:
			if di.argCount() == 9 {
				startY := di.y
				pt1x := di.x + di.evalArg(0)
				pt1y := di.y + di.evalArg(1)
				pt2x := pt1x + di.evalArg(2)
				pt2y := pt1y + di.evalArg(3)
				pt3x := pt2x + di.evalArg(4)
				pt3y := pt2y
				di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

				pt4x := di.x + di.evalArg(5)
				pt4y := di.y
				pt5x := pt4x + di.evalArg(6)
				pt5y := pt4y + di.evalArg(7)
				pt6x := pt5x + di.evalArg(8)
				pt6y := startY
				di.emitCubicTo(pt4x, pt4y, pt5x, pt5y, pt6x, pt6y)
			}
			di.clearArgs()

		// HarfBuzz: path_procs_t::flex1 in hb-cff-interp-cs-common.hh:828-863
		case csFlex1:
			if di.argCount() == 11 {
				// Calculate total delta to determine final direction.
				var dx, dy float64
				for i := 0; i < 10; i += 2 {
					dx += di.evalArg(i)
					dy += di.evalArg(i + 1)
				}

				startX, startY := di.x, di.y
				pt1x := di.x + di.evalArg(0)
				pt1y := di.y + di.evalArg(1)
				pt2x := pt1x + di.evalArg(2)
				pt2y := pt1y + di.evalArg(3)
				pt3x := pt2x + di.evalArg(4)
				pt3y := pt2y + di.evalArg(5)
				di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

				pt4x := di.x + di.evalArg(6)
				pt4y := di.y + di.evalArg(7)
				pt5x := pt4x + di.evalArg(8)
				pt5y := pt4y + di.evalArg(9)
				var pt6x, pt6y float64
				if math.Abs(dx) > math.Abs(dy) {
					pt6x = pt5x + di.evalArg(10)
					pt6y = startY
				} else {
					pt6x = startX
					pt6y = pt5y + di.evalArg(10)
				}
				di.emitCubicTo(pt4x, pt4y, pt5x, pt5y, pt6x, pt6y)
			}
			di.clearArgs()

		// --- Subroutine operators ---
		// HarfBuzz: cs_opset_t::process_op OpCode_callsubr/callgsubr
		// hb-cff-interp-cs-common.hh:259-265
		case csCallsubr:
			if len(di.stack) > 0 {
				subrNum := int(di.popArg()) + di.localBias
				if subrNum >= 0 && subrNum < len(di.localSubrs) && di.callDepth < cffMaxCallDepth {
					di.callDepth++
					di.execute(di.localSubrs[subrNum])
					di.callDepth--
				}
			}

		case csCallgsubr:
			if len(di.stack) > 0 {
				subrNum := int(di.popArg()) + di.globalBias
				if subrNum >= 0 && subrNum < len(di.globalSubrs) && di.callDepth < cffMaxCallDepth {
					di.callDepth++
					di.execute(di.globalSubrs[subrNum])
					di.callDepth--
				}
			}

		case csReturn:
			// Return from subroutine.
			return

		case csEndchar:
			// End of CharString.
			// HarfBuzz: cff1_cs_opset_t::process_op OpCode_endchar
			// hb-cff1-interp-cs.hh:96-104
			di.checkWidth(op)
			di.clearArgs()
			return

		default:
			// Unknown operator — clear stack.
			di.clearArgs()
		}
	}
}

// emitMoveTo adds a MoveTo segment at the current point.
func (di *cffDrawInterpreter) emitMoveTo() {
	di.segments = append(di.segments, Segment{
		Op:   SegmentMoveTo,
		Args: [3]OutlinePoint{{float32(di.x), float32(di.y)}},
	})
}

// emitLineTo adds a LineTo segment at the current point.
func (di *cffDrawInterpreter) emitLineTo() {
	di.segments = append(di.segments, Segment{
		Op:   SegmentLineTo,
		Args: [3]OutlinePoint{{float32(di.x), float32(di.y)}},
	})
}

// emitCubicTo adds a CubeTo segment and updates the current point.
// HarfBuzz equivalent: PATH::curve in cff1_path_procs_path_t (hb-ot-cff1-table.cc:504-511)
func (di *cffDrawInterpreter) emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y float64) {
	di.segments = append(di.segments, Segment{
		Op: SegmentCubeTo,
		Args: [3]OutlinePoint{
			{float32(pt1x), float32(pt1y)},
			{float32(pt2x), float32(pt2y)},
			{float32(pt3x), float32(pt3y)},
		},
	})
	di.x = pt3x
	di.y = pt3y
}

// emitRRCurveTo is a convenience for the common rrcurveto pattern (6 relative deltas).
func (di *cffDrawInterpreter) emitRRCurveTo(dxa, dya, dxb, dyb, dxc, dyc float64) {
	pt1x := di.x + dxa
	pt1y := di.y + dya
	pt2x := pt1x + dxb
	pt2y := pt1y + dyb
	pt3x := pt2x + dxc
	pt3y := pt2y + dyc
	di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)
}

// processAlternatingCurves handles vhcurveto and hvcurveto operators.
// When startVertical is true, this is vhcurveto; when false, hvcurveto.
// HarfBuzz equivalent: path_procs_t::vhcurveto/hvcurveto in hb-cff-interp-cs-common.hh:625-744
func (di *cffDrawInterpreter) processAlternatingCurves(startVertical bool) {
	ac := di.argCount()
	i := 0

	if (ac % 8) >= 4 {
		// First group of 4 args.
		var pt1x, pt1y, pt2x, pt2y, pt3x, pt3y float64
		if startVertical {
			pt1x = di.x
			pt1y = di.y + di.evalArg(i)
			pt2x = pt1x + di.evalArg(i+1)
			pt2y = pt1y + di.evalArg(i+2)
			pt3x = pt2x + di.evalArg(i+3)
			pt3y = pt2y
		} else {
			pt1x = di.x + di.evalArg(i)
			pt1y = di.y
			pt2x = pt1x + di.evalArg(i+1)
			pt2y = pt1y + di.evalArg(i+2)
			pt3x = pt2x
			pt3y = pt2y + di.evalArg(i+3)
		}
		i += 4

		for i+8 <= ac {
			di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

			// Alternate direction group.
			if startVertical {
				pt1x = di.x + di.evalArg(i)
				pt1y = di.y
				pt2x = pt1x + di.evalArg(i+1)
				pt2y = pt1y + di.evalArg(i+2)
				pt3x = pt2x
				pt3y = pt2y + di.evalArg(i+3)
			} else {
				pt1x = di.x
				pt1y = di.y + di.evalArg(i)
				pt2x = pt1x + di.evalArg(i+1)
				pt2y = pt1y + di.evalArg(i+2)
				pt3x = pt2x + di.evalArg(i+3)
				pt3y = pt2y
			}
			di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

			// Back to original direction.
			if startVertical {
				pt1x = di.x
				pt1y = di.y + di.evalArg(i+4)
				pt2x = pt1x + di.evalArg(i+5)
				pt2y = pt1y + di.evalArg(i+6)
				pt3x = pt2x + di.evalArg(i+7)
				pt3y = pt2y
			} else {
				pt1x = di.x + di.evalArg(i+4)
				pt1y = di.y
				pt2x = pt1x + di.evalArg(i+5)
				pt2y = pt1y + di.evalArg(i+6)
				pt3x = pt2x
				pt3y = pt2y + di.evalArg(i+7)
			}
			i += 8
		}
		// Final trailing arg adjusts the "other" coordinate.
		if i < ac {
			if startVertical {
				pt3y += di.evalArg(i)
			} else {
				pt3x += di.evalArg(i)
			}
		}
		di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)
	} else {
		for i+8 <= ac {
			// First direction group.
			var pt1x, pt1y, pt2x, pt2y, pt3x, pt3y float64
			if startVertical {
				pt1x = di.x
				pt1y = di.y + di.evalArg(i)
				pt2x = pt1x + di.evalArg(i+1)
				pt2y = pt1y + di.evalArg(i+2)
				pt3x = pt2x + di.evalArg(i+3)
				pt3y = pt2y
			} else {
				pt1x = di.x + di.evalArg(i)
				pt1y = di.y
				pt2x = pt1x + di.evalArg(i+1)
				pt2y = pt1y + di.evalArg(i+2)
				pt3x = pt2x
				pt3y = pt2y + di.evalArg(i+3)
			}
			di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

			// Alternate direction group.
			if startVertical {
				pt1x = di.x + di.evalArg(i+4)
				pt1y = di.y
				pt2x = pt1x + di.evalArg(i+5)
				pt2y = pt1y + di.evalArg(i+6)
				pt3x = pt2x
				pt3y = pt2y + di.evalArg(i+7)
			} else {
				pt1x = di.x
				pt1y = di.y + di.evalArg(i+4)
				pt2x = pt1x + di.evalArg(i+5)
				pt2y = pt1y + di.evalArg(i+6)
				pt3x = pt2x + di.evalArg(i+7)
				pt3y = pt2y
			}
			// Trailing arg on last pair.
			if (ac-i < 16) && ((ac & 1) != 0) {
				if startVertical {
					pt3x += di.evalArg(i + 8)
				} else {
					pt3y += di.evalArg(i + 8)
				}
			}
			di.emitCubicTo(pt1x, pt1y, pt2x, pt2y, pt3x, pt3y)

			i += 8
		}
	}
}

// decodeCSOperandFloat decodes a CharString operand as float64.
// Unlike decodeCSOperand (which returns int), this preserves
// full precision for 16.16 fixed-point numbers (byte 255).
// HarfBuzz equivalent: number_t encoding in hb-cff-interp-common.hh:563-633
func decodeCSOperandFloat(data []byte) (float64, int) {
	if len(data) == 0 {
		return 0, 0
	}

	b0 := data[0]

	// 1-byte integer (32-246)
	if b0 >= 32 && b0 <= 246 {
		return float64(int(b0) - 139), 1
	}

	// 2-byte positive integer (247-250)
	if b0 >= 247 && b0 <= 250 {
		if len(data) < 2 {
			return 0, 1
		}
		return float64((int(b0)-247)*256 + int(data[1]) + 108), 2
	}

	// 2-byte negative integer (251-254)
	if b0 >= 251 && b0 <= 254 {
		if len(data) < 2 {
			return 0, 1
		}
		return float64(-(int(b0)-251)*256 - int(data[1]) - 108), 2
	}

	// 3-byte integer (operator 28)
	if b0 == 28 {
		if len(data) < 3 {
			return 0, 1
		}
		v := int16(binary.BigEndian.Uint16(data[1:3]))
		return float64(v), 3
	}

	// 16.16 fixed-point number (operator 255).
	// HarfBuzz: number_t::set_fixed(v) { value = v / 65536.0; }
	// in hb-cff-interp-common.hh:231
	if b0 == 255 {
		if len(data) < 5 {
			return 0, 1
		}
		v := int32(binary.BigEndian.Uint32(data[1:5]))
		return float64(v) / 65536.0, 5
	}

	return 0, 1
}
