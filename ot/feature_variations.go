package ot

// FeatureVariations table implementation for GSUB/GPOS version 1.1+
//
// HarfBuzz reference:
//   - hb-ot-layout-common.hh:4641 - FeatureVariations struct
//   - hb-ot-layout-common.hh:4305 - ConditionSet struct
//   - hb-ot-layout-common.hh:3912 - ConditionAxisRange (Format 1)
//   - hb-ot-shape.hh:36 - hb_ot_shape_plan_key_t with variations_index

import (
	"encoding/binary"
)

// VariationsNotFoundIndex indicates no matching FeatureVariations record was found.
// HarfBuzz: HB_OT_LAYOUT_NO_VARIATIONS_INDEX
const VariationsNotFoundIndex = 0xFFFFFFFF

// FeatureVariations represents the FeatureVariations table in GSUB/GPOS v1.1+.
// It allows fonts to substitute feature lookups based on variation axis values.
type FeatureVariations struct {
	data    []byte
	offset  int
	records []featureVariationRecord
}

// featureVariationRecord pairs a ConditionSet with a FeatureTableSubstitution.
type featureVariationRecord struct {
	conditionSetOffset     uint32
	featureSubstOffset     uint32
	conditionSet           *ConditionSet
	featureTableSubst      *FeatureTableSubstitution
}

// ConditionSet represents a set of conditions that must ALL be true (AND logic).
// HarfBuzz: struct ConditionSet in hb-ot-layout-common.hh:4305
type ConditionSet struct {
	data       []byte
	offset     int
	conditions []*Condition
}

// Condition represents a single condition.
// Currently only Format 1 (ConditionAxisRange) is defined.
// HarfBuzz: struct Condition in hb-ot-layout-common.hh:4209
type Condition struct {
	format         uint16
	axisIndex      uint16
	filterRangeMin int16 // F2DOT14
	filterRangeMax int16 // F2DOT14
}

// FeatureTableSubstitution maps feature indices to alternate lookup lists.
// HarfBuzz: struct FeatureTableSubstitution in hb-ot-layout-common.hh:4479
type FeatureTableSubstitution struct {
	data    []byte
	offset  int
	version uint32
	records []featureSubstitutionRecord
}

// featureSubstitutionRecord maps a feature index to an alternate feature.
type featureSubstitutionRecord struct {
	featureIndex  uint16
	lookupIndices []uint16
}

// ParseFeatureVariations parses a FeatureVariations table from data at the given offset.
func ParseFeatureVariations(data []byte, offset int) (*FeatureVariations, error) {
	if offset+8 > len(data) {
		return nil, ErrInvalidOffset
	}

	// Version (major.minor)
	major := binary.BigEndian.Uint16(data[offset:])
	minor := binary.BigEndian.Uint16(data[offset+2:])
	if major != 1 || minor != 0 {
		return nil, ErrInvalidFormat
	}

	recordCount := int(binary.BigEndian.Uint32(data[offset+4:]))
	if offset+8+recordCount*8 > len(data) {
		return nil, ErrInvalidOffset
	}

	fv := &FeatureVariations{
		data:    data,
		offset:  offset,
		records: make([]featureVariationRecord, recordCount),
	}

	// Parse each FeatureVariationRecord
	for i := 0; i < recordCount; i++ {
		recOff := offset + 8 + i*8
		condSetOff := binary.BigEndian.Uint32(data[recOff:])
		featSubstOff := binary.BigEndian.Uint32(data[recOff+4:])

		fv.records[i] = featureVariationRecord{
			conditionSetOffset: condSetOff,
			featureSubstOffset: featSubstOff,
		}

		// Parse ConditionSet
		if condSetOff != 0 {
			cs, err := parseConditionSet(data, offset+int(condSetOff))
			if err == nil {
				fv.records[i].conditionSet = cs
			}
		}

		// Parse FeatureTableSubstitution
		if featSubstOff != 0 {
			fts, err := parseFeatureTableSubstitution(data, offset+int(featSubstOff))
			if err == nil {
				fv.records[i].featureTableSubst = fts
			}
		}
	}

	return fv, nil
}

// parseConditionSet parses a ConditionSet table.
func parseConditionSet(data []byte, offset int) (*ConditionSet, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	condCount := int(binary.BigEndian.Uint16(data[offset:]))
	if offset+2+condCount*4 > len(data) {
		return nil, ErrInvalidOffset
	}

	cs := &ConditionSet{
		data:       data,
		offset:     offset,
		conditions: make([]*Condition, 0, condCount),
	}

	// Parse each Condition offset and the condition itself
	for i := 0; i < condCount; i++ {
		condOff := int(binary.BigEndian.Uint32(data[offset+2+i*4:]))
		cond, err := parseCondition(data, offset+condOff)
		if err != nil {
			continue
		}
		cs.conditions = append(cs.conditions, cond)
	}

	return cs, nil
}

// parseCondition parses a Condition table (currently only Format 1: AxisRange).
func parseCondition(data []byte, offset int) (*Condition, error) {
	if offset+2 > len(data) {
		return nil, ErrInvalidOffset
	}

	format := binary.BigEndian.Uint16(data[offset:])

	switch format {
	case 1:
		// Format 1: ConditionAxisRange
		if offset+8 > len(data) {
			return nil, ErrInvalidOffset
		}
		return &Condition{
			format:         format,
			axisIndex:      binary.BigEndian.Uint16(data[offset+2:]),
			filterRangeMin: int16(binary.BigEndian.Uint16(data[offset+4:])),
			filterRangeMax: int16(binary.BigEndian.Uint16(data[offset+6:])),
		}, nil

	default:
		// Unknown format - treat as always false
		return &Condition{format: format}, nil
	}
}

// parseFeatureTableSubstitution parses a FeatureTableSubstitution table.
func parseFeatureTableSubstitution(data []byte, offset int) (*FeatureTableSubstitution, error) {
	if offset+6 > len(data) {
		return nil, ErrInvalidOffset
	}

	major := binary.BigEndian.Uint16(data[offset:])
	minor := binary.BigEndian.Uint16(data[offset+2:])
	version := uint32(major)<<16 | uint32(minor)

	if major != 1 || minor != 0 {
		return nil, ErrInvalidFormat
	}

	substCount := int(binary.BigEndian.Uint16(data[offset+4:]))
	if offset+6+substCount*6 > len(data) {
		return nil, ErrInvalidOffset
	}

	fts := &FeatureTableSubstitution{
		data:    data,
		offset:  offset,
		version: version,
		records: make([]featureSubstitutionRecord, substCount),
	}

	// Parse each substitution record
	for i := 0; i < substCount; i++ {
		recOff := offset + 6 + i*6
		featureIndex := binary.BigEndian.Uint16(data[recOff:])
		altFeatureOff := binary.BigEndian.Uint32(data[recOff+2:])

		fts.records[i].featureIndex = featureIndex

		// Parse the alternate Feature table to get lookup indices
		if altFeatureOff != 0 {
			lookups, err := parseAlternateFeature(data, offset+int(altFeatureOff))
			if err == nil {
				fts.records[i].lookupIndices = lookups
			}
		}
	}

	return fts, nil
}

// parseAlternateFeature parses an alternate Feature table and returns its lookup indices.
// The alternate Feature table has the same format as a regular Feature table.
func parseAlternateFeature(data []byte, offset int) ([]uint16, error) {
	if offset+4 > len(data) {
		return nil, ErrInvalidOffset
	}

	// Feature table format:
	// uint16 featureParamsOffset (usually 0)
	// uint16 lookupIndexCount
	// uint16[lookupIndexCount] lookupListIndices

	// Skip featureParamsOffset
	lookupCount := int(binary.BigEndian.Uint16(data[offset+2:]))
	if offset+4+lookupCount*2 > len(data) {
		return nil, ErrInvalidOffset
	}

	lookups := make([]uint16, lookupCount)
	for i := 0; i < lookupCount; i++ {
		lookups[i] = binary.BigEndian.Uint16(data[offset+4+i*2:])
	}

	return lookups, nil
}

// FindIndex finds the first matching FeatureVariationRecord for the given
// normalized coordinates (in F2DOT14 format). Returns VariationsNotFoundIndex
// if no record matches.
// HarfBuzz: FeatureVariations::find_index() in hb-ot-layout-common.hh:4690
func (fv *FeatureVariations) FindIndex(coords []int) uint32 {
	if fv == nil || len(fv.records) == 0 {
		return VariationsNotFoundIndex
	}

	for i, rec := range fv.records {
		if rec.conditionSet != nil && rec.conditionSet.Evaluate(coords) {
			return uint32(i)
		}
	}

	return VariationsNotFoundIndex
}

// Evaluate returns true if ALL conditions in the set are satisfied.
// HarfBuzz: ConditionSet::evaluate() in hb-ot-layout-common.hh:4340
func (cs *ConditionSet) Evaluate(coords []int) bool {
	if cs == nil || len(cs.conditions) == 0 {
		return false
	}

	for _, cond := range cs.conditions {
		if !cond.Evaluate(coords) {
			return false
		}
	}

	return true
}

// Evaluate returns true if the condition is satisfied for the given coordinates.
// HarfBuzz: Condition::evaluate() / ConditionAxisRange::evaluate()
func (c *Condition) Evaluate(coords []int) bool {
	if c == nil {
		return false
	}

	switch c.format {
	case 1:
		// Format 1: ConditionAxisRange
		// Check if coord[axisIndex] is within [filterRangeMin, filterRangeMax]
		if int(c.axisIndex) >= len(coords) {
			return false
		}
		coordValue := int16(coords[c.axisIndex])
		return coordValue >= c.filterRangeMin && coordValue <= c.filterRangeMax

	default:
		// Unknown format - treat as not satisfied
		return false
	}
}

// GetSubstituteLookups returns the substitute lookup indices for a feature at the
// given variationsIndex. Returns nil if no substitution exists for this feature.
// HarfBuzz: FeatureTableSubstitution::find_substitute() in hb-ot-layout-common.hh:4585
func (fv *FeatureVariations) GetSubstituteLookups(variationsIndex uint32, featureIndex uint16) []uint16 {
	if fv == nil || variationsIndex == VariationsNotFoundIndex {
		return nil
	}

	if int(variationsIndex) >= len(fv.records) {
		return nil
	}

	fts := fv.records[variationsIndex].featureTableSubst
	if fts == nil {
		return nil
	}

	// Search for the feature index in the substitution records
	for _, rec := range fts.records {
		if rec.featureIndex == featureIndex {
			return rec.lookupIndices
		}
	}

	return nil
}

// GetRecord returns the number of FeatureVariationRecords.
func (fv *FeatureVariations) RecordCount() int {
	if fv == nil {
		return 0
	}
	return len(fv.records)
}
