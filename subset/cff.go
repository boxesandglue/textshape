package subset

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/boxesandglue/textshape/ot"
)

// sidRemap handles SID remapping like HarfBuzz's remap_sid_t.
// Standard SIDs (0-390) are unchanged, custom SIDs are remapped to new consecutive values.
type sidRemap struct {
	mapping map[int]int // old custom SID index -> new custom SID index
	strings []string    // original strings in new order
	next    int         // next available custom SID index
}

const numStdStrings = 391

func newSIDRemap() *sidRemap {
	return &sidRemap{
		mapping: make(map[int]int),
		next:    0,
	}
}

// add adds a SID to the remap and returns the new SID.
// Standard SIDs (< 391) are returned unchanged.
func (r *sidRemap) add(sid int, origStrings []string) int {
	if sid < numStdStrings || sid == 0 {
		return sid
	}

	// Convert to string index (0-based index into custom strings)
	strIdx := sid - numStdStrings

	// Check if already mapped
	if newIdx, ok := r.mapping[strIdx]; ok {
		return newIdx + numStdStrings
	}

	// Add new mapping
	newIdx := r.next
	r.mapping[strIdx] = newIdx
	r.next++

	// Store the string
	if strIdx >= 0 && strIdx < len(origStrings) {
		r.strings = append(r.strings, origStrings[strIdx])
	} else {
		r.strings = append(r.strings, "")
	}

	return newIdx + numStdStrings
}

// get returns the remapped SID for an original SID.
func (r *sidRemap) get(sid int) int {
	if sid < numStdStrings || sid == 0 {
		return sid
	}

	strIdx := sid - numStdStrings
	if newIdx, ok := r.mapping[strIdx]; ok {
		return newIdx + numStdStrings
	}
	return sid // shouldn't happen if add was called first
}

// subrRemap handles subroutine remapping like HarfBuzz's subr_remap_t.
// Maps old subroutine numbers to new consecutive numbers.
type subrRemap struct {
	mapping map[int]int // old subr number -> new subr number
	count   int         // number of remapped subrs
	bias    int         // bias for new subr set
}

func newSubrRemap() *subrRemap {
	return &subrRemap{
		mapping: make(map[int]int),
	}
}

// create builds the remap from a closure set (like HarfBuzz).
func (r *subrRemap) create(closure map[int]bool) {
	if len(closure) == 0 {
		r.bias = 107
		return
	}

	// Sort the used subroutine numbers
	nums := make([]int, 0, len(closure))
	for n := range closure {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Create consecutive mapping
	for newNum, oldNum := range nums {
		r.mapping[oldNum] = newNum
	}
	r.count = len(nums)

	// Calculate bias for new subr count
	if r.count < 1240 {
		r.bias = 107
	} else if r.count < 33900 {
		r.bias = 1131
	} else {
		r.bias = 32768
	}
}

// biasedNum returns the biased number for encoding in CharString.
func (r *subrRemap) biasedNum(oldNum int) int {
	if newNum, ok := r.mapping[oldNum]; ok {
		return newNum - r.bias
	}
	return 0
}

// has checks if a subroutine number is in the remap.
func (r *subrRemap) has(oldNum int) bool {
	_, ok := r.mapping[oldNum]
	return ok
}

// collectSubrClosure collects all subroutines used by the given CharStrings.
// This is like HarfBuzz's subr_subsetter_t::collect_subrs.
func collectSubrClosure(charStrings [][]byte, globalSubrs, localSubrs [][]byte) (globalClosure, localClosure map[int]bool) {
	globalClosure = make(map[int]bool)
	localClosure = make(map[int]bool)

	globalBias := calcSubrBias(len(globalSubrs))
	localBias := calcSubrBias(len(localSubrs))

	// Process each CharString
	for _, cs := range charStrings {
		collectSubrsFromCharString(cs, globalSubrs, localSubrs, globalBias, localBias, globalClosure, localClosure, make(map[int]bool), make(map[int]bool))
	}

	return globalClosure, localClosure
}

// collectSubrsFromCharString recursively collects subroutine calls from a CharString.
func collectSubrsFromCharString(data []byte, globalSubrs, localSubrs [][]byte,
	globalBias, localBias int, globalClosure, localClosure map[int]bool,
	visitedGlobal, visitedLocal map[int]bool) {

	pos := 0
	stack := make([]int, 0, 48)

	for pos < len(data) {
		b := data[pos]

		// Number encoding
		if b >= 32 && b <= 246 {
			stack = append(stack, int(b)-139)
			pos++
		} else if b >= 247 && b <= 250 {
			if pos+1 >= len(data) {
				break
			}
			stack = append(stack, (int(b)-247)*256+int(data[pos+1])+108)
			pos += 2
		} else if b >= 251 && b <= 254 {
			if pos+1 >= len(data) {
				break
			}
			stack = append(stack, -(int(b)-251)*256-int(data[pos+1])-108)
			pos += 2
		} else if b == 28 {
			if pos+2 >= len(data) {
				break
			}
			stack = append(stack, int(int16(binary.BigEndian.Uint16(data[pos+1:]))))
			pos += 3
		} else if b == 255 {
			// Fixed-point number (CFF2 / Type 2)
			if pos+4 >= len(data) {
				break
			}
			pos += 5
		} else {
			// Operator
			op := int(b)
			pos++

			if b == 12 && pos < len(data) {
				op = 12<<8 | int(data[pos])
				pos++
			}

			switch op {
			case 10: // callsubr (local)
				if len(stack) > 0 {
					biasedNum := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					subrNum := biasedNum + localBias

					if subrNum >= 0 && subrNum < len(localSubrs) && !visitedLocal[subrNum] {
						localClosure[subrNum] = true
						visitedLocal[subrNum] = true
						// Recursively process the subroutine
						collectSubrsFromCharString(localSubrs[subrNum], globalSubrs, localSubrs,
							globalBias, localBias, globalClosure, localClosure, visitedGlobal, visitedLocal)
					}
				}
			case 29: // callgsubr (global)
				if len(stack) > 0 {
					biasedNum := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					subrNum := biasedNum + globalBias

					if subrNum >= 0 && subrNum < len(globalSubrs) && !visitedGlobal[subrNum] {
						globalClosure[subrNum] = true
						visitedGlobal[subrNum] = true
						// Recursively process the subroutine
						collectSubrsFromCharString(globalSubrs[subrNum], globalSubrs, localSubrs,
							globalBias, localBias, globalClosure, localClosure, visitedGlobal, visitedLocal)
					}
				}
			case 11: // return
				return
			case 14: // endchar
				return
			default:
				// Clear stack for most operators (simplified)
				stack = stack[:0]
			}
		}
	}
}

// remapCharStringSubrs rewrites a CharString with new subroutine numbers.
func remapCharStringSubrs(data []byte, globalRemap, localRemap *subrRemap, oldGlobalBias, oldLocalBias int) []byte {
	var result bytes.Buffer
	pos := 0
	stack := make([]int, 0, 48)
	stackPositions := make([]int, 0, 48) // byte positions where each stack value started

	for pos < len(data) {
		startPos := pos
		b := data[pos]

		// Number encoding
		if b >= 32 && b <= 246 {
			stack = append(stack, int(b)-139)
			stackPositions = append(stackPositions, result.Len())
			result.WriteByte(b)
			pos++
		} else if b >= 247 && b <= 250 {
			if pos+1 >= len(data) {
				break
			}
			stack = append(stack, (int(b)-247)*256+int(data[pos+1])+108)
			stackPositions = append(stackPositions, result.Len())
			result.Write(data[pos : pos+2])
			pos += 2
		} else if b >= 251 && b <= 254 {
			if pos+1 >= len(data) {
				break
			}
			stack = append(stack, -(int(b)-251)*256-int(data[pos+1])-108)
			stackPositions = append(stackPositions, result.Len())
			result.Write(data[pos : pos+2])
			pos += 2
		} else if b == 28 {
			if pos+2 >= len(data) {
				break
			}
			stack = append(stack, int(int16(binary.BigEndian.Uint16(data[pos+1:]))))
			stackPositions = append(stackPositions, result.Len())
			result.Write(data[pos : pos+3])
			pos += 3
		} else if b == 255 {
			if pos+4 >= len(data) {
				break
			}
			result.Write(data[pos : pos+5])
			pos += 5
		} else {
			// Operator
			op := int(b)
			pos++

			if b == 12 && pos < len(data) {
				op = 12<<8 | int(data[pos])
				pos++
			}

			switch op {
			case 10: // callsubr (local)
				if len(stack) > 0 && localRemap != nil {
					biasedNum := stack[len(stack)-1]
					oldSubrNum := biasedNum + oldLocalBias

					if localRemap.has(oldSubrNum) {
						// Rewrite the number with new bias
						newBiasedNum := localRemap.biasedNum(oldSubrNum)
						// Truncate result to before the last number
						result.Truncate(stackPositions[len(stackPositions)-1])
						// Write new number
						result.Write(encodeCharStringInt(newBiasedNum))
					}
					stack = stack[:len(stack)-1]
					stackPositions = stackPositions[:len(stackPositions)-1]
				}
				result.Write(data[startPos:pos])

			case 29: // callgsubr (global)
				if len(stack) > 0 && globalRemap != nil {
					biasedNum := stack[len(stack)-1]
					oldSubrNum := biasedNum + oldGlobalBias

					if globalRemap.has(oldSubrNum) {
						// Rewrite the number with new bias
						newBiasedNum := globalRemap.biasedNum(oldSubrNum)
						// Truncate result to before the last number
						result.Truncate(stackPositions[len(stackPositions)-1])
						// Write new number
						result.Write(encodeCharStringInt(newBiasedNum))
					}
					stack = stack[:len(stack)-1]
					stackPositions = stackPositions[:len(stackPositions)-1]
				}
				result.Write(data[startPos:pos])

			default:
				// Copy operator as-is
				result.Write(data[startPos:pos])
				stack = stack[:0]
				stackPositions = stackPositions[:0]
			}
		}
	}

	return result.Bytes()
}

// encodeCharStringInt encodes an integer for CharString format.
func encodeCharStringInt(v int) []byte {
	if v >= -107 && v <= 107 {
		return []byte{byte(v + 139)}
	}
	if v >= 108 && v <= 1131 {
		v -= 108
		return []byte{byte(v/256 + 247), byte(v % 256)}
	}
	if v >= -1131 && v <= -108 {
		v = -v - 108
		return []byte{byte(v/256 + 251), byte(v % 256)}
	}
	// Use 3-byte encoding for larger numbers
	return []byte{28, byte(v >> 8), byte(v)}
}

// subsetCFF creates a subsetted CFF table.
// This implementation follows HarfBuzz's approach:
// - SIDs are remapped so only used strings are included
// - Only used subroutines are kept (like HarfBuzz's subr_subsetter_t)
// - CharStrings are rewritten with new subroutine numbers
func (p *Plan) subsetCFF() ([]byte, error) {
	cff := p.cff
	if cff == nil {
		return nil, nil
	}

	// 1. Create SID remapping (like HarfBuzz's remap_sid_t)
	sidmap := newSIDRemap()

	// 2. Collect SIDs from TopDict first (like HarfBuzz: collect_sids_in_dicts)
	newTopDictSIDs := topDictSIDs{
		Version:    sidmap.add(cff.TopDict.Version, cff.Strings),
		Notice:     sidmap.add(cff.TopDict.Notice, cff.Strings),
		FullName:   sidmap.add(cff.TopDict.FullName, cff.Strings),
		FamilyName: sidmap.add(cff.TopDict.FamilyName, cff.Strings),
		Weight:     sidmap.add(cff.TopDict.Weight, cff.Strings),
	}

	// 3. Collect CharStrings for kept glyphs
	// Like HarfBuzz: only glyphs in glyphSet get their original CharString,
	// all other slots (padding for FlagRetainGIDs) get just endchar
	usedCharStrings := make([][]byte, p.numOutputGlyphs)
	subsetCharStrings := make([][]byte, 0, len(p.glyphSet))

	for newGID := 0; newGID < p.numOutputGlyphs; newGID++ {
		oldGID, exists := p.reverseMap[ot.GlyphID(newGID)]
		if !exists {
			oldGID = ot.GlyphID(newGID) // FlagRetainGIDs: oldGID == newGID
		}

		// Only include actual CharString if glyph is in the subset
		if p.glyphSet[oldGID] && int(oldGID) < len(cff.CharStrings) {
			usedCharStrings[newGID] = cff.CharStrings[oldGID]
			subsetCharStrings = append(subsetCharStrings, cff.CharStrings[oldGID])
		} else {
			// Padding slot or out-of-range: just endchar (like HarfBuzz)
			usedCharStrings[newGID] = []byte{14} // endchar
		}
	}

	// 4. Collect subroutine closure ONLY from glyphs actually in the subset
	// (like HarfBuzz's subr_subsetter_t::collect_subrs)
	globalClosure, localClosure := collectSubrClosure(subsetCharStrings, cff.GlobalSubrs, cff.LocalSubrs)

	// 5. Create subroutine remaps (like HarfBuzz's subr_remap_t)
	globalRemap := newSubrRemap()
	globalRemap.create(globalClosure)
	localRemap := newSubrRemap()
	localRemap.create(localClosure)

	// Calculate old biases
	oldGlobalBias := calcSubrBias(len(cff.GlobalSubrs))
	oldLocalBias := calcSubrBias(len(cff.LocalSubrs))

	// 6. Remap CharStrings with new subroutine numbers
	for i := range usedCharStrings {
		usedCharStrings[i] = remapCharStringSubrs(usedCharStrings[i], globalRemap, localRemap, oldGlobalBias, oldLocalBias)
	}

	// 7. Build new subroutine arrays (only used ones, in order)
	newGlobalSubrs := extractUsedSubrs(cff.GlobalSubrs, globalClosure, globalRemap, localRemap, oldGlobalBias, oldLocalBias)
	newLocalSubrs := extractUsedSubrs(cff.LocalSubrs, localClosure, globalRemap, localRemap, oldGlobalBias, oldLocalBias)

	// 8. Build new Charset with remapped SIDs
	newCharset := buildCFFCharsetWithRemap(cff.Charset, p.reverseMap, p.numOutputGlyphs, sidmap, cff.Strings)

	// 9. Serialize with remapped SIDs
	return serializeCFFWithSIDMap(cff, usedCharStrings, newGlobalSubrs, newLocalSubrs, newCharset, sidmap, newTopDictSIDs)
}

// extractUsedSubrs extracts and remaps subroutines that are in the closure.
func extractUsedSubrs(subrs [][]byte, closure map[int]bool, globalRemap, localRemap *subrRemap, oldGlobalBias, oldLocalBias int) [][]byte {
	if len(closure) == 0 {
		return nil
	}

	// Sort by old number to get new consecutive order
	nums := make([]int, 0, len(closure))
	for n := range closure {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Extract and remap subroutines
	result := make([][]byte, len(nums))
	for newNum, oldNum := range nums {
		if oldNum >= 0 && oldNum < len(subrs) {
			// Remap subr calls within the subroutine
			result[newNum] = remapCharStringSubrs(subrs[oldNum], globalRemap, localRemap, oldGlobalBias, oldLocalBias)
		} else {
			result[newNum] = []byte{11} // return
		}
	}
	return result
}

// buildSubrMap creates a mapping from old subroutine numbers to new (consecutive) numbers.
func buildSubrMap(used map[int]bool) map[int]int {
	if len(used) == 0 {
		return nil
	}

	// Sort used subroutine numbers
	nums := make([]int, 0, len(used))
	for n := range used {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Create consecutive mapping
	m := make(map[int]int, len(nums))
	for i, n := range nums {
		m[n] = i
	}
	return m
}

// subsetSubrs extracts and remaps used subroutines.
// subrMap is the mapping for THIS type of subroutine.
// globalMap/localMap are for remapping calls WITHIN the subroutines.
func subsetSubrs(subrs [][]byte, subrMap map[int]int,
	globalMap, localMap map[int]int,
	oldGlobalBias, oldLocalBias, newGlobalBias, newLocalBias int) [][]byte {
	if len(subrMap) == 0 {
		return nil
	}

	// Sort by new index
	type entry struct {
		oldNum int
		newNum int
	}
	entries := make([]entry, 0, len(subrMap))
	for old, new := range subrMap {
		entries = append(entries, entry{old, new})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].newNum < entries[j].newNum
	})

	result := make([][]byte, len(entries))
	for _, e := range entries {
		if e.oldNum >= 0 && e.oldNum < len(subrs) {
			// Remap subroutine calls within the subroutine
			// RemapCharString expects (globalMap, localMap) order
			remapped := ot.RemapCharString(subrs[e.oldNum], globalMap, localMap,
				oldGlobalBias, oldLocalBias, newGlobalBias, newLocalBias)
			result[e.newNum] = remapped
		} else {
			result[e.newNum] = []byte{11} // return
		}
	}
	return result
}

// charsetRange represents a range in Format 1/2 charset.
type charsetRange struct {
	first int // first SID in range
	nLeft int // number of remaining SIDs (count - 1)
}

// buildCFFCharsetWithRemap builds an optimized charset with SID remapping (like HarfBuzz).
// Automatically chooses Format 0, 1, or 2 based on which is smallest.
func buildCFFCharsetWithRemap(origCharset []ot.GlyphID, reverseMap map[ot.GlyphID]ot.GlyphID, numGlyphs int, sidmap *sidRemap, origStrings []string) []byte {
	if numGlyphs <= 1 {
		return []byte{0} // Format 0, only .notdef
	}

	// 1. Collect all SIDs (remapped)
	sids := make([]int, numGlyphs-1)
	for newGID := 1; newGID < numGlyphs; newGID++ {
		oldGID, exists := reverseMap[ot.GlyphID(newGID)]
		if !exists {
			oldGID = ot.GlyphID(newGID) // FlagRetainGIDs
		}

		var sid int
		if int(oldGID) < len(origCharset) {
			origSID := int(origCharset[oldGID])
			sid = sidmap.add(origSID, origStrings)
		} else {
			sid = newGID
		}
		sids[newGID-1] = sid
	}

	// 2. Build ranges (for Format 1/2)
	var ranges []charsetRange
	needsTwoBytes := false
	i := 0
	for i < len(sids) {
		first := sids[i]
		count := 1
		for i+count < len(sids) && sids[i+count] == first+count {
			count++
		}
		nLeft := count - 1
		if nLeft > 255 {
			needsTwoBytes = true
		}
		ranges = append(ranges, charsetRange{first: first, nLeft: nLeft})
		i += count
	}

	// 3. Calculate sizes for each format
	format0Size := 1 + (numGlyphs-1)*2
	format1Size := 1 + len(ranges)*3 // first(2) + nLeft(1)
	format2Size := 1 + len(ranges)*4 // first(2) + nLeft(2)

	// 4. Choose smallest format (like HarfBuzz)
	// Format 1 can only be used if all nLeft values fit in 1 byte
	if !needsTwoBytes && format1Size < format0Size {
		// Use Format 1
		return buildCharsetFormat1(ranges)
	} else if format2Size < format0Size {
		// Use Format 2
		return buildCharsetFormat2(ranges)
	}
	// Use Format 0
	return buildCharsetFormat0(sids)
}

// buildCharsetFormat0 builds a Format 0 charset (array of SIDs).
func buildCharsetFormat0(sids []int) []byte {
	buf := make([]byte, 1+len(sids)*2)
	buf[0] = 0 // Format 0
	for i, sid := range sids {
		binary.BigEndian.PutUint16(buf[1+i*2:], uint16(sid))
	}
	return buf
}

// buildCharsetFormat1 builds a Format 1 charset (ranges with 1-byte nLeft).
func buildCharsetFormat1(ranges []charsetRange) []byte {
	buf := make([]byte, 1+len(ranges)*3)
	buf[0] = 1 // Format 1
	for i, r := range ranges {
		binary.BigEndian.PutUint16(buf[1+i*3:], uint16(r.first))
		buf[1+i*3+2] = byte(r.nLeft)
	}
	return buf
}

// buildCharsetFormat2 builds a Format 2 charset (ranges with 2-byte nLeft).
func buildCharsetFormat2(ranges []charsetRange) []byte {
	buf := make([]byte, 1+len(ranges)*4)
	buf[0] = 2 // Format 2
	for i, r := range ranges {
		binary.BigEndian.PutUint16(buf[1+i*4:], uint16(r.first))
		binary.BigEndian.PutUint16(buf[1+i*4+2:], uint16(r.nLeft))
	}
	return buf
}

// topDictSIDs holds the remapped SIDs for TopDict fields.
type topDictSIDs struct {
	Version    int
	Notice     int
	FullName   int
	FamilyName int
	Weight     int
}

// serializeCFFWithSIDMap writes the complete CFF table with SID remapping (like HarfBuzz).
func serializeCFFWithSIDMap(original *ot.CFF, charStrings [][]byte,
	globalSubrs, localSubrs [][]byte, charset []byte,
	sidmap *sidRemap, topSIDs topDictSIDs) ([]byte, error) {

	var buf bytes.Buffer

	// Phase 1: Calculate sizes and offsets
	headerSize := 4

	// Name INDEX
	nameData := [][]byte{[]byte(original.Name)}
	nameINDEX := buildINDEX(nameData)

	// String INDEX - now contains the remapped custom strings
	stringData := make([][]byte, len(sidmap.strings))
	for i, s := range sidmap.strings {
		stringData[i] = []byte(s)
	}
	stringINDEX := buildINDEX(stringData)

	// Global Subrs INDEX
	globalSubrsINDEX := buildINDEX(globalSubrs)

	// CharStrings INDEX
	charStringsINDEX := buildINDEX(charStrings)

	// Private DICT
	var privateDict bytes.Buffer
	writePrivateDict(&privateDict, &original.PrivateDict, false, 0)

	// Local Subrs offset calculation
	localSubrsINDEX := buildINDEX(localSubrs)
	subrsOperatorSize := 0
	if len(localSubrs) > 0 {
		currentLen := privateDict.Len()
		estimatedOffset := currentLen + 3
		encodedSize := len(encodeCFFInt(estimatedOffset)) + 1
		actualOffset := currentLen + encodedSize
		if actualOffset != estimatedOffset {
			encodedSize = len(encodeCFFInt(actualOffset)) + 1
		}
		subrsOperatorSize = encodedSize
	}

	privateDictSize := privateDict.Len() + subrsOperatorSize

	// Build Top DICT with remapped SIDs (like HarfBuzz)
	topDictData := buildTopDictWithSIDs(original, topSIDs, 0, 0, privateDictSize, 0)

	// Estimate Top DICT INDEX size
	estimatedTopDictSize := len(topDictData)
	topDictINDEXSize := 2 + 1 + 2 + estimatedTopDictSize // count + offSize + offsets + data

	// Calculate offsets
	offset := headerSize
	offset += len(nameINDEX)
	offset += topDictINDEXSize
	offset += len(stringINDEX)
	offset += len(globalSubrsINDEX)

	charsetOffset := offset
	offset += len(charset)

	charStringsOffset := offset
	offset += len(charStringsINDEX)

	privateDictOffset := offset

	// Rebuild Top DICT with correct offsets
	topDictData = buildTopDictWithSIDs(original, topSIDs, charsetOffset, charStringsOffset, privateDictSize, privateDictOffset)
	topDictINDEX := buildINDEX([][]byte{topDictData})

	// Recalculate if size changed
	if len(topDictINDEX) != topDictINDEXSize {
		sizeDiff := len(topDictINDEX) - topDictINDEXSize

		charsetOffset += sizeDiff
		charStringsOffset += sizeDiff
		privateDictOffset += sizeDiff

		topDictData = buildTopDictWithSIDs(original, topSIDs, charsetOffset, charStringsOffset, privateDictSize, privateDictOffset)
		topDictINDEX = buildINDEX([][]byte{topDictData})
	}

	// Phase 2: Write the CFF data

	// Header (like HarfBuzz: offSize = 4)
	buf.WriteByte(1) // major
	buf.WriteByte(0) // minor
	buf.WriteByte(4) // hdrSize
	buf.WriteByte(4) // offSize

	// Name INDEX
	buf.Write(nameINDEX)

	// Top DICT INDEX
	buf.Write(topDictINDEX)

	// String INDEX
	buf.Write(stringINDEX)

	// Global Subrs INDEX
	buf.Write(globalSubrsINDEX)

	// Charset
	buf.Write(charset)

	// CharStrings INDEX
	buf.Write(charStringsINDEX)

	// Private DICT - write with correct Subrs offset
	privateDict.Reset()
	subrsOffset := 0
	if len(localSubrs) > 0 {
		writePrivateDict(&privateDict, &original.PrivateDict, false, 0)
		baseSize := privateDict.Len()

		estimatedOffset := baseSize + 3
		encodedSize := len(encodeCFFInt(estimatedOffset)) + 1
		actualOffset := baseSize + encodedSize
		if actualOffset != estimatedOffset {
			encodedSize = len(encodeCFFInt(actualOffset)) + 1
			actualOffset = baseSize + encodedSize
		}
		subrsOffset = actualOffset

		privateDict.Reset()
		writePrivateDict(&privateDict, &original.PrivateDict, true, subrsOffset)
	} else {
		writePrivateDict(&privateDict, &original.PrivateDict, false, 0)
	}
	buf.Write(privateDict.Bytes())

	// Local Subrs INDEX
	if len(localSubrs) > 0 {
		buf.Write(localSubrsINDEX)
	}

	return buf.Bytes(), nil
}

// buildTopDictWithSIDs creates a Top DICT with remapped SIDs (like HarfBuzz).
func buildTopDictWithSIDs(original *ot.CFF, sids topDictSIDs, charsetOff, charStringsOff, privateSize, privateOff int) []byte {
	var buf bytes.Buffer

	// Write SID entries (like HarfBuzz's cff1_top_dict_op_serializer_t)
	if sids.Version != 0 {
		writeDictInt(&buf, sids.Version, 0) // version
	}
	if sids.Notice != 0 {
		writeDictInt(&buf, sids.Notice, 1) // Notice
	}
	if sids.FullName != 0 {
		writeDictInt(&buf, sids.FullName, 2) // FullName
	}
	if sids.FamilyName != 0 {
		writeDictInt(&buf, sids.FamilyName, 3) // FamilyName
	}
	if sids.Weight != 0 {
		writeDictInt(&buf, sids.Weight, 4) // Weight
	}

	// FontBBox (operator 5) - REQUIRED
	writeIntArray(&buf, original.TopDict.FontBBox[:], 5)

	// charset (operator 15)
	writeDictInt(&buf, charsetOff, 15)

	// CharStrings (operator 17)
	writeDictInt(&buf, charStringsOff, 17)

	// Private (operator 18) - size and offset
	buf.Write(encodeCFFInt(privateSize))
	buf.Write(encodeCFFInt(privateOff))
	buf.WriteByte(18)

	return buf.Bytes()
}

// buildINDEX creates a CFF INDEX structure.
func buildINDEX(data [][]byte) []byte {
	count := len(data)

	if count == 0 {
		// Empty INDEX: just count = 0
		return []byte{0, 0}
	}

	// Calculate total data size
	totalSize := 0
	for _, d := range data {
		totalSize += len(d)
	}

	// Determine offset size
	offSize := 1
	if totalSize+1 > 255 {
		offSize = 2
	}
	if totalSize+1 > 65535 {
		offSize = 3
	}
	if totalSize+1 > 16777215 {
		offSize = 4
	}

	// Build INDEX
	// count(2) + offSize(1) + offsets((count+1)*offSize) + data
	indexSize := 2 + 1 + (count+1)*offSize + totalSize
	buf := make([]byte, indexSize)

	// Count
	binary.BigEndian.PutUint16(buf[0:], uint16(count))
	// OffSize
	buf[2] = byte(offSize)

	// Offsets (1-based)
	offset := 1
	for i := 0; i <= count; i++ {
		writeOffset(buf[3+i*offSize:], offset, offSize)
		if i < count {
			offset += len(data[i])
		}
	}

	// Data
	dataStart := 3 + (count+1)*offSize
	pos := dataStart
	for _, d := range data {
		copy(buf[pos:], d)
		pos += len(d)
	}

	return buf
}

func writeOffset(buf []byte, offset, size int) {
	switch size {
	case 1:
		buf[0] = byte(offset)
	case 2:
		binary.BigEndian.PutUint16(buf, uint16(offset))
	case 3:
		buf[0] = byte(offset >> 16)
		buf[1] = byte(offset >> 8)
		buf[2] = byte(offset)
	case 4:
		binary.BigEndian.PutUint32(buf, uint32(offset))
	}
}

// buildTopDict creates a Top DICT with the given entries.
func buildTopDict(entries map[int][]int) []byte {
	var buf bytes.Buffer

	// Write entries in a consistent order
	keys := make([]int, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	for _, op := range keys {
		vals := entries[op]
		for _, v := range vals {
			buf.Write(encodeCFFInt(v))
		}
		if op >= 256 {
			buf.WriteByte(12)
			buf.WriteByte(byte(op & 0xff))
		} else {
			buf.WriteByte(byte(op))
		}
	}

	return buf.Bytes()
}

// writeDictInt writes an integer operand followed by an operator.
func writeDictInt(buf *bytes.Buffer, val int, op int) {
	buf.Write(encodeCFFInt(val))
	if op >= 256 {
		buf.WriteByte(12)
		buf.WriteByte(byte(op & 0xff))
	} else {
		buf.WriteByte(byte(op))
	}
}

// writeIntArray writes an array of integers followed by an operator.
func writeIntArray(buf *bytes.Buffer, vals []int, op int) {
	for _, v := range vals {
		buf.Write(encodeCFFInt(v))
	}
	if op >= 256 {
		buf.WriteByte(12)
		buf.WriteByte(byte(op & 0xff))
	} else {
		buf.WriteByte(byte(op))
	}
}

// encodeCFFInt encodes an integer in CFF DICT format.
func encodeCFFInt(v int) []byte {
	if v >= -107 && v <= 107 {
		return []byte{byte(v + 139)}
	}
	if v >= 108 && v <= 1131 {
		v -= 108
		return []byte{byte(v/256 + 247), byte(v % 256)}
	}
	if v >= -1131 && v <= -108 {
		v = -v - 108
		return []byte{byte(v/256 + 251), byte(v % 256)}
	}
	if v >= -32768 && v <= 32767 {
		return []byte{28, byte(v >> 8), byte(v)}
	}
	// 4-byte integer
	return []byte{29, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

// calcSubrBias calculates subroutine bias based on count.
func calcSubrBias(count int) int {
	if count < 1240 {
		return 107
	}
	if count < 33900 {
		return 1131
	}
	return 32768
}

// writePrivateDict writes all Private DICT fields like HarfBuzz does.
// This ensures maximum compatibility with PDF renderers.
func writePrivateDict(buf *bytes.Buffer, pd *ot.PrivateDict, includeSubrs bool, subrsOffset int) {
	// BlueValues (op 6)
	if len(pd.BlueValues) > 0 {
		writeIntArray(buf, pd.BlueValues, 6)
	}
	// OtherBlues (op 7)
	if len(pd.OtherBlues) > 0 {
		writeIntArray(buf, pd.OtherBlues, 7)
	}
	// FamilyBlues (op 8) - HarfBuzz copies this
	if len(pd.FamilyBlues) > 0 {
		writeIntArray(buf, pd.FamilyBlues, 8)
	}
	// FamilyOtherBlues (op 9) - HarfBuzz copies this
	if len(pd.FamilyOtherBlues) > 0 {
		writeIntArray(buf, pd.FamilyOtherBlues, 9)
	}
	// BlueScale (op 12 9) - HarfBuzz copies this if non-default
	// Default is 0.039625, we skip for now since it's a float
	// BlueFuzz (op 12 11) - HarfBuzz copies this if non-default
	if pd.BlueFuzz != 1 { // 1 is default
		writeDictInt(buf, pd.BlueFuzz, 12<<8|11)
	}
	// StdHW (op 10)
	if pd.StdHW != 0 {
		writeDictInt(buf, pd.StdHW, 10)
	}
	// StdVW (op 11)
	if pd.StdVW != 0 {
		writeDictInt(buf, pd.StdVW, 11)
	}
	// StemSnapH (op 12 12) - HarfBuzz copies this
	if len(pd.StemSnapH) > 0 {
		writeIntArray(buf, pd.StemSnapH, 12<<8|12)
	}
	// StemSnapV (op 12 13) - HarfBuzz copies this
	if len(pd.StemSnapV) > 0 {
		writeIntArray(buf, pd.StemSnapV, 12<<8|13)
	}
	// defaultWidthX (op 20) - only if non-zero
	if pd.DefaultWidthX != 0 {
		writeDictInt(buf, pd.DefaultWidthX, 20)
	}
	// nominalWidthX (op 21)
	if pd.NominalWidthX != 0 {
		writeDictInt(buf, pd.NominalWidthX, 21)
	}
	// Subrs (op 19) - offset to local subroutines
	if includeSubrs && subrsOffset > 0 {
		writeDictInt(buf, subrsOffset, 19)
	}
}
