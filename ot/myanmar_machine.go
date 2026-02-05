//line myanmar_machine.rl:1
// Copyright 2011,2012 Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-myanmar-machine.rl

package ot

//line myanmar_machine.go:12
const myanmarSyllableMachine_start int = 0
const myanmarSyllableMachine_first_final int = 0
const myanmarSyllableMachine_error int = -1

const myanmarSyllableMachine_en_main int = 0

//line myanmar_machine.rl:12

//line myanmar_machine.rl:69

// FindSyllablesMyanmar finds Myanmar syllable boundaries in the buffer.
// It sets the syllable field in Info[i].Syllable to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_myanmar() in hb-ot-shaper-myanmar-machine.hh
func FindSyllablesMyanmar(buf *Buffer, categories []MyanmarCategory) (hasBroken bool) {
	if len(buf.Info) == 0 {
		return false
	}

	n := len(buf.Info)
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		data[i] = byte(categories[i])
	}

	var cs, p, pe, eof, ts, te, act int
	_ = act // Suppress unused variable warning

	pe = n
	eof = pe

	var syllableSerial uint8 = 1

	foundSyllable := func(syllableType MyanmarSyllableType) {
		for i := ts; i < te; i++ {
			buf.Info[i].Syllable = (syllableSerial << 4) | uint8(syllableType)
		}
		syllableSerial++
		if syllableSerial == 16 {
			syllableSerial = 1
		}
	}

//line myanmar_machine.go:60
	{
		cs = myanmarSyllableMachine_start
		ts = 0
		te = 0
		act = 0
	}

//line myanmar_machine.go:68
	{
		if p == pe {
			goto _test_eof
		}
		switch cs {
		case 0:
			goto st_case_0
		case 1:
			goto st_case_1
		case 2:
			goto st_case_2
		case 3:
			goto st_case_3
		case 4:
			goto st_case_4
		case 5:
			goto st_case_5
		case 6:
			goto st_case_6
		case 7:
			goto st_case_7
		case 8:
			goto st_case_8
		case 9:
			goto st_case_9
		case 10:
			goto st_case_10
		case 11:
			goto st_case_11
		case 12:
			goto st_case_12
		case 13:
			goto st_case_13
		case 14:
			goto st_case_14
		case 15:
			goto st_case_15
		case 16:
			goto st_case_16
		case 17:
			goto st_case_17
		case 18:
			goto st_case_18
		case 19:
			goto st_case_19
		case 20:
			goto st_case_20
		case 21:
			goto st_case_21
		case 22:
			goto st_case_22
		case 23:
			goto st_case_23
		case 24:
			goto st_case_24
		case 25:
			goto st_case_25
		case 26:
			goto st_case_26
		case 27:
			goto st_case_27
		case 28:
			goto st_case_28
		case 29:
			goto st_case_29
		case 30:
			goto st_case_30
		case 31:
			goto st_case_31
		case 32:
			goto st_case_32
		case 33:
			goto st_case_33
		case 34:
			goto st_case_34
		case 35:
			goto st_case_35
		case 36:
			goto st_case_36
		case 37:
			goto st_case_37
		case 38:
			goto st_case_38
		case 39:
			goto st_case_39
		case 40:
			goto st_case_40
		case 41:
			goto st_case_41
		case 42:
			goto st_case_42
		case 43:
			goto st_case_43
		case 44:
			goto st_case_44
		case 45:
			goto st_case_45
		case 46:
			goto st_case_46
		case 47:
			goto st_case_47
		case 48:
			goto st_case_48
		case 49:
			goto st_case_49
		case 50:
			goto st_case_50
		case 51:
			goto st_case_51
		case 52:
			goto st_case_52
		}
		goto st_out
	tr0:
//line myanmar_machine.rl:66
		te = p + 1
		{
			foundSyllable(MyanmarNonMyanmarCluster)
		}
		goto st0
	tr4:
//line myanmar_machine.rl:64
		te = p + 1
		{
			foundSyllable(MyanmarNonMyanmarCluster)
		}
		goto st0
	tr22:
//line myanmar_machine.rl:63
		te = p
		p--
		{
			foundSyllable(MyanmarConsonantSyllable)
		}
		goto st0
	tr25:
//line myanmar_machine.rl:63
		te = p + 1
		{
			foundSyllable(MyanmarConsonantSyllable)
		}
		goto st0
	tr47:
//line myanmar_machine.rl:65
		te = p
		p--
		{
			foundSyllable(MyanmarBrokenCluster)
			hasBroken = true
		}
		goto st0
	tr48:
//line myanmar_machine.rl:65
		te = p + 1
		{
			foundSyllable(MyanmarBrokenCluster)
			hasBroken = true
		}
		goto st0
	tr50:
//line NONE:1
		switch act {
		case 2:
			{
				p = (te) - 1
				foundSyllable(MyanmarNonMyanmarCluster)
			}
		case 3:
			{
				p = (te) - 1
				foundSyllable(MyanmarBrokenCluster)
				hasBroken = true
			}
		}

		goto st0
	tr60:
//line myanmar_machine.rl:66
		te = p
		p--
		{
			foundSyllable(MyanmarNonMyanmarCluster)
		}
		goto st0
	st0:
//line NONE:1
		ts = 0

		if p++; p == pe {
			goto _test_eof0
		}
	st_case_0:
//line NONE:1
		ts = p

//line myanmar_machine.go:243
		switch data[p] {
		case 3:
			goto st25
		case 4:
			goto st35
		case 8:
			goto tr5
		case 9:
			goto st30
		case 15:
			goto st49
		case 18:
			goto st52
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st41
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 38:
			goto st46
		case 39:
			goto st27
		case 40:
			goto st48
		case 41:
			goto st43
		case 57:
			goto tr21
		}
		switch {
		case data[p] < 5:
			if 1 <= data[p] && data[p] <= 2 {
				goto st1
			}
		case data[p] > 6:
			if 10 <= data[p] && data[p] <= 11 {
				goto st1
			}
		default:
			goto tr4
		}
		goto tr0
	st1:
		if p++; p == pe {
			goto _test_eof1
		}
	st_case_1:
		switch data[p] {
		case 3:
			goto st2
		case 4:
			goto st12
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st17
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 38:
			goto st22
		case 39:
			goto st4
		case 40:
			goto st24
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st2:
		if p++; p == pe {
			goto _test_eof2
		}
	st_case_2:
		switch data[p] {
		case 8:
			goto st3
		case 23:
			goto st6
		case 32:
			goto st11
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st3:
		if p++; p == pe {
			goto _test_eof3
		}
	st_case_3:
		switch data[p] {
		case 8:
			goto st3
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st4:
		if p++; p == pe {
			goto _test_eof4
		}
	st_case_4:
		switch data[p] {
		case 3:
			goto st5
		case 8:
			goto st3
		case 9:
			goto st4
		case 32:
			goto st3
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st5:
		if p++; p == pe {
			goto _test_eof5
		}
	st_case_5:
		switch data[p] {
		case 8:
			goto st3
		case 32:
			goto st3
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st6:
		if p++; p == pe {
			goto _test_eof6
		}
	st_case_6:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st8
		case 23:
			goto st6
		case 32:
			goto st9
		case 35:
			goto st10
		case 39:
			goto st4
		case 41:
			goto st9
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st7:
		if p++; p == pe {
			goto _test_eof7
		}
	st_case_7:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 23:
			goto st6
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st8:
		if p++; p == pe {
			goto _test_eof8
		}
	st_case_8:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st8
		case 23:
			goto st6
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st9:
		if p++; p == pe {
			goto _test_eof9
		}
	st_case_9:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st8
		case 23:
			goto st6
		case 32:
			goto st9
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st10:
		if p++; p == pe {
			goto _test_eof10
		}
	st_case_10:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st8
		case 23:
			goto st6
		case 32:
			goto st9
		case 39:
			goto st4
		case 41:
			goto st9
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st11:
		if p++; p == pe {
			goto _test_eof11
		}
	st_case_11:
		switch data[p] {
		case 8:
			goto st3
		case 23:
			goto st6
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st12:
		if p++; p == pe {
			goto _test_eof12
		}
	st_case_12:
		if data[p] == 15 {
			goto st1
		}
		if 1 <= data[p] && data[p] <= 2 {
			goto st1
		}
		goto tr22
	st13:
		if p++; p == pe {
			goto _test_eof13
		}
	st_case_13:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 23:
			goto st6
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st14:
		if p++; p == pe {
			goto _test_eof14
		}
	st_case_14:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 21:
			goto st14
		case 23:
			goto st6
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st15:
		if p++; p == pe {
			goto _test_eof15
		}
	st_case_15:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 39:
			goto st4
		case 40:
			goto st16
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st16:
		if p++; p == pe {
			goto _test_eof16
		}
	st_case_16:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st17:
		if p++; p == pe {
			goto _test_eof17
		}
	st_case_17:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st17
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 38:
			goto st22
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st18:
		if p++; p == pe {
			goto _test_eof18
		}
	st_case_18:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st16
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st19:
		if p++; p == pe {
			goto _test_eof19
		}
	st_case_19:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st16
		case 39:
			goto st4
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st20:
		if p++; p == pe {
			goto _test_eof20
		}
	st_case_20:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 35:
			goto st18
		case 37:
			goto st21
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st21:
		if p++; p == pe {
			goto _test_eof21
		}
	st_case_21:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st16
		case 35:
			goto st18
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st22:
		if p++; p == pe {
			goto _test_eof22
		}
	st_case_22:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st23
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st23:
		if p++; p == pe {
			goto _test_eof23
		}
	st_case_23:
		switch data[p] {
		case 3:
			goto st2
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st24:
		if p++; p == pe {
			goto _test_eof24
		}
	st_case_24:
		switch data[p] {
		case 3:
			goto st2
		case 4:
			goto st12
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st17
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 38:
			goto st22
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st25:
		if p++; p == pe {
			goto _test_eof25
		}
	st_case_25:
		switch data[p] {
		case 8:
			goto tr5
		case 23:
			goto st29
		case 32:
			goto st34
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	tr5:
//line NONE:1
		te = p + 1

//line myanmar_machine.rl:65
		act = 3
		goto st26
	tr21:
//line NONE:1
		te = p + 1

//line myanmar_machine.rl:64
		act = 2
		goto st26
	st26:
		if p++; p == pe {
			goto _test_eof26
		}
	st_case_26:
//line myanmar_machine.go:1034
		switch data[p] {
		case 8:
			goto tr5
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr50
	st27:
		if p++; p == pe {
			goto _test_eof27
		}
	st_case_27:
		switch data[p] {
		case 3:
			goto st28
		case 8:
			goto tr5
		case 9:
			goto st27
		case 32:
			goto tr5
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st28:
		if p++; p == pe {
			goto _test_eof28
		}
	st_case_28:
		switch data[p] {
		case 8:
			goto tr5
		case 32:
			goto tr5
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st29:
		if p++; p == pe {
			goto _test_eof29
		}
	st_case_29:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st31
		case 23:
			goto st29
		case 32:
			goto st32
		case 35:
			goto st33
		case 39:
			goto st27
		case 41:
			goto st32
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st30:
		if p++; p == pe {
			goto _test_eof30
		}
	st_case_30:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 23:
			goto st29
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st31:
		if p++; p == pe {
			goto _test_eof31
		}
	st_case_31:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st31
		case 23:
			goto st29
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st32:
		if p++; p == pe {
			goto _test_eof32
		}
	st_case_32:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st31
		case 23:
			goto st29
		case 32:
			goto st32
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st33:
		if p++; p == pe {
			goto _test_eof33
		}
	st_case_33:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st31
		case 23:
			goto st29
		case 32:
			goto st32
		case 39:
			goto st27
		case 41:
			goto st32
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st34:
		if p++; p == pe {
			goto _test_eof34
		}
	st_case_34:
		switch data[p] {
		case 8:
			goto tr5
		case 23:
			goto st29
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st35:
		if p++; p == pe {
			goto _test_eof35
		}
	st_case_35:
		if data[p] == 15 {
			goto st36
		}
		if 1 <= data[p] && data[p] <= 2 {
			goto st36
		}
		goto tr47
	st36:
		if p++; p == pe {
			goto _test_eof36
		}
	st_case_36:
		switch data[p] {
		case 3:
			goto st25
		case 4:
			goto st35
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st41
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 38:
			goto st46
		case 39:
			goto st27
		case 40:
			goto st48
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st37:
		if p++; p == pe {
			goto _test_eof37
		}
	st_case_37:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 23:
			goto st29
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st38:
		if p++; p == pe {
			goto _test_eof38
		}
	st_case_38:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 21:
			goto st38
		case 23:
			goto st29
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st39:
		if p++; p == pe {
			goto _test_eof39
		}
	st_case_39:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 39:
			goto st27
		case 40:
			goto st40
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st40:
		if p++; p == pe {
			goto _test_eof40
		}
	st_case_40:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st41:
		if p++; p == pe {
			goto _test_eof41
		}
	st_case_41:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st41
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 38:
			goto st46
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st42:
		if p++; p == pe {
			goto _test_eof42
		}
	st_case_42:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st40
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st43:
		if p++; p == pe {
			goto _test_eof43
		}
	st_case_43:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st40
		case 39:
			goto st27
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st44:
		if p++; p == pe {
			goto _test_eof44
		}
	st_case_44:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 35:
			goto st42
		case 37:
			goto st45
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st45:
		if p++; p == pe {
			goto _test_eof45
		}
	st_case_45:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st40
		case 35:
			goto st42
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st46:
		if p++; p == pe {
			goto _test_eof46
		}
	st_case_46:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st47
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st47:
		if p++; p == pe {
			goto _test_eof47
		}
	st_case_47:
		switch data[p] {
		case 3:
			goto st25
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st48:
		if p++; p == pe {
			goto _test_eof48
		}
	st_case_48:
		switch data[p] {
		case 3:
			goto st25
		case 4:
			goto st35
		case 8:
			goto tr5
		case 9:
			goto st30
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st41
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 38:
			goto st46
		case 39:
			goto st27
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr48
		}
		goto tr47
	st49:
		if p++; p == pe {
			goto _test_eof49
		}
	st_case_49:
		switch data[p] {
		case 3:
			goto st2
		case 4:
			goto st12
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st50
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 38:
			goto st22
		case 39:
			goto st4
		case 40:
			goto st24
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st50:
		if p++; p == pe {
			goto _test_eof50
		}
	st_case_50:
		switch data[p] {
		case 3:
			goto st2
		case 4:
			goto st51
		case 8:
			goto st3
		case 9:
			goto st7
		case 20:
			goto st13
		case 21:
			goto st14
		case 22:
			goto st15
		case 23:
			goto st6
		case 32:
			goto st17
		case 35:
			goto st18
		case 36:
			goto st20
		case 37:
			goto st21
		case 38:
			goto st22
		case 39:
			goto st4
		case 41:
			goto st19
		case 57:
			goto st3
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto tr25
		}
		goto tr22
	st51:
		if p++; p == pe {
			goto _test_eof51
		}
	st_case_51:
		switch data[p] {
		case 3:
			goto st25
		case 4:
			goto st35
		case 8:
			goto tr5
		case 9:
			goto st30
		case 15:
			goto st1
		case 20:
			goto st37
		case 21:
			goto st38
		case 22:
			goto st39
		case 23:
			goto st29
		case 32:
			goto st41
		case 35:
			goto st42
		case 36:
			goto st44
		case 37:
			goto st45
		case 38:
			goto st46
		case 39:
			goto st27
		case 40:
			goto st48
		case 41:
			goto st43
		case 57:
			goto tr5
		}
		switch {
		case data[p] < 5:
			if 1 <= data[p] && data[p] <= 2 {
				goto st1
			}
		case data[p] > 6:
			if 10 <= data[p] && data[p] <= 11 {
				goto st1
			}
		default:
			goto tr48
		}
		goto tr47
	st52:
		if p++; p == pe {
			goto _test_eof52
		}
	st_case_52:
		if data[p] == 15 {
			goto st1
		}
		switch {
		case data[p] > 2:
			if 10 <= data[p] && data[p] <= 11 {
				goto st1
			}
		case data[p] >= 1:
			goto st1
		}
		goto tr60
	st_out:
	_test_eof0:
		cs = 0
		goto _test_eof
	_test_eof1:
		cs = 1
		goto _test_eof
	_test_eof2:
		cs = 2
		goto _test_eof
	_test_eof3:
		cs = 3
		goto _test_eof
	_test_eof4:
		cs = 4
		goto _test_eof
	_test_eof5:
		cs = 5
		goto _test_eof
	_test_eof6:
		cs = 6
		goto _test_eof
	_test_eof7:
		cs = 7
		goto _test_eof
	_test_eof8:
		cs = 8
		goto _test_eof
	_test_eof9:
		cs = 9
		goto _test_eof
	_test_eof10:
		cs = 10
		goto _test_eof
	_test_eof11:
		cs = 11
		goto _test_eof
	_test_eof12:
		cs = 12
		goto _test_eof
	_test_eof13:
		cs = 13
		goto _test_eof
	_test_eof14:
		cs = 14
		goto _test_eof
	_test_eof15:
		cs = 15
		goto _test_eof
	_test_eof16:
		cs = 16
		goto _test_eof
	_test_eof17:
		cs = 17
		goto _test_eof
	_test_eof18:
		cs = 18
		goto _test_eof
	_test_eof19:
		cs = 19
		goto _test_eof
	_test_eof20:
		cs = 20
		goto _test_eof
	_test_eof21:
		cs = 21
		goto _test_eof
	_test_eof22:
		cs = 22
		goto _test_eof
	_test_eof23:
		cs = 23
		goto _test_eof
	_test_eof24:
		cs = 24
		goto _test_eof
	_test_eof25:
		cs = 25
		goto _test_eof
	_test_eof26:
		cs = 26
		goto _test_eof
	_test_eof27:
		cs = 27
		goto _test_eof
	_test_eof28:
		cs = 28
		goto _test_eof
	_test_eof29:
		cs = 29
		goto _test_eof
	_test_eof30:
		cs = 30
		goto _test_eof
	_test_eof31:
		cs = 31
		goto _test_eof
	_test_eof32:
		cs = 32
		goto _test_eof
	_test_eof33:
		cs = 33
		goto _test_eof
	_test_eof34:
		cs = 34
		goto _test_eof
	_test_eof35:
		cs = 35
		goto _test_eof
	_test_eof36:
		cs = 36
		goto _test_eof
	_test_eof37:
		cs = 37
		goto _test_eof
	_test_eof38:
		cs = 38
		goto _test_eof
	_test_eof39:
		cs = 39
		goto _test_eof
	_test_eof40:
		cs = 40
		goto _test_eof
	_test_eof41:
		cs = 41
		goto _test_eof
	_test_eof42:
		cs = 42
		goto _test_eof
	_test_eof43:
		cs = 43
		goto _test_eof
	_test_eof44:
		cs = 44
		goto _test_eof
	_test_eof45:
		cs = 45
		goto _test_eof
	_test_eof46:
		cs = 46
		goto _test_eof
	_test_eof47:
		cs = 47
		goto _test_eof
	_test_eof48:
		cs = 48
		goto _test_eof
	_test_eof49:
		cs = 49
		goto _test_eof
	_test_eof50:
		cs = 50
		goto _test_eof
	_test_eof51:
		cs = 51
		goto _test_eof
	_test_eof52:
		cs = 52
		goto _test_eof

	_test_eof:
		{
		}
		if p == eof {
			switch cs {
			case 1:
				goto tr22
			case 2:
				goto tr22
			case 3:
				goto tr22
			case 4:
				goto tr22
			case 5:
				goto tr22
			case 6:
				goto tr22
			case 7:
				goto tr22
			case 8:
				goto tr22
			case 9:
				goto tr22
			case 10:
				goto tr22
			case 11:
				goto tr22
			case 12:
				goto tr22
			case 13:
				goto tr22
			case 14:
				goto tr22
			case 15:
				goto tr22
			case 16:
				goto tr22
			case 17:
				goto tr22
			case 18:
				goto tr22
			case 19:
				goto tr22
			case 20:
				goto tr22
			case 21:
				goto tr22
			case 22:
				goto tr22
			case 23:
				goto tr22
			case 24:
				goto tr22
			case 25:
				goto tr47
			case 26:
				goto tr50
			case 27:
				goto tr47
			case 28:
				goto tr47
			case 29:
				goto tr47
			case 30:
				goto tr47
			case 31:
				goto tr47
			case 32:
				goto tr47
			case 33:
				goto tr47
			case 34:
				goto tr47
			case 35:
				goto tr47
			case 36:
				goto tr47
			case 37:
				goto tr47
			case 38:
				goto tr47
			case 39:
				goto tr47
			case 40:
				goto tr47
			case 41:
				goto tr47
			case 42:
				goto tr47
			case 43:
				goto tr47
			case 44:
				goto tr47
			case 45:
				goto tr47
			case 46:
				goto tr47
			case 47:
				goto tr47
			case 48:
				goto tr47
			case 49:
				goto tr22
			case 50:
				goto tr22
			case 51:
				goto tr47
			case 52:
				goto tr60
			}
		}

	}

//line myanmar_machine.rl:106

	_ = cs            // Suppress unused variable warning
	_ = foundSyllable // May be unused if no syllables found

	return hasBroken
}
