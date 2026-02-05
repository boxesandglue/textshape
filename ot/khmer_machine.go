//line khmer_machine.rl:1
// Copyright 2011,2012 Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-khmer-machine.rl

package ot

//line khmer_machine.go:12
const khmerSyllableMachine_start int = 21
const khmerSyllableMachine_first_final int = 21
const khmerSyllableMachine_error int = -1

const khmerSyllableMachine_en_main int = 21

//line khmer_machine.rl:12

//line khmer_machine.rl:61

// FindSyllablesKhmer finds Khmer syllable boundaries in the buffer.
// It sets the syllable field in Info[i].Mask to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_khmer() in hb-ot-shaper-khmer-machine.hh
func FindSyllablesKhmer(buf *Buffer, categories []KhmerCategory) (hasBroken bool) {
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

	foundSyllable := func(syllableType KhmerSyllableType) {
		syllableValue := uint32(syllableSerial<<4) | uint32(syllableType)
		for i := ts; i < te; i++ {
			buf.Info[i].Mask = (buf.Info[i].Mask & 0xFFFF0000) | syllableValue
		}
		syllableSerial++
		if syllableSerial == 16 {
			syllableSerial = 1
		}
	}

//line khmer_machine.go:61
	{
		cs = khmerSyllableMachine_start
		ts = 0
		te = 0
		act = 0
	}

//line khmer_machine.go:69
	{
		if p == pe {
			goto _test_eof
		}
		switch cs {
		case 21:
			goto st_case_21
		case 22:
			goto st_case_22
		case 23:
			goto st_case_23
		case 24:
			goto st_case_24
		case 0:
			goto st_case_0
		case 1:
			goto st_case_1
		case 25:
			goto st_case_25
		case 2:
			goto st_case_2
		case 26:
			goto st_case_26
		case 3:
			goto st_case_3
		case 27:
			goto st_case_27
		case 4:
			goto st_case_4
		case 28:
			goto st_case_28
		case 5:
			goto st_case_5
		case 29:
			goto st_case_29
		case 6:
			goto st_case_6
		case 7:
			goto st_case_7
		case 30:
			goto st_case_30
		case 8:
			goto st_case_8
		case 9:
			goto st_case_9
		case 31:
			goto st_case_31
		case 10:
			goto st_case_10
		case 32:
			goto st_case_32
		case 33:
			goto st_case_33
		case 34:
			goto st_case_34
		case 11:
			goto st_case_11
		case 12:
			goto st_case_12
		case 35:
			goto st_case_35
		case 13:
			goto st_case_13
		case 36:
			goto st_case_36
		case 14:
			goto st_case_14
		case 37:
			goto st_case_37
		case 15:
			goto st_case_15
		case 38:
			goto st_case_38
		case 16:
			goto st_case_16
		case 39:
			goto st_case_39
		case 17:
			goto st_case_17
		case 18:
			goto st_case_18
		case 40:
			goto st_case_40
		case 19:
			goto st_case_19
		case 20:
			goto st_case_20
		case 41:
			goto st_case_41
		case 42:
			goto st_case_42
		}
		goto st_out
	tr0:
//line khmer_machine.rl:55
		p = (te) - 1
		{
			foundSyllable(KhmerConsonantSyllable)
		}
		goto st21
	tr14:
//line khmer_machine.rl:56
		p = (te) - 1
		{
			foundSyllable(KhmerBrokenCluster)
			hasBroken = true
		}
		goto st21
	tr19:
//line NONE:1
		switch act {
		case 2:
			{
				p = (te) - 1
				foundSyllable(KhmerBrokenCluster)
				hasBroken = true
			}
		case 3:
			{
				p = (te) - 1
				foundSyllable(KhmerNonKhmerCluster)
			}
		}

		goto st21
	tr28:
//line khmer_machine.rl:57
		te = p + 1
		{
			foundSyllable(KhmerNonKhmerCluster)
		}
		goto st21
	tr32:
//line khmer_machine.rl:55
		te = p
		p--
		{
			foundSyllable(KhmerConsonantSyllable)
		}
		goto st21
	tr41:
//line khmer_machine.rl:56
		te = p
		p--
		{
			foundSyllable(KhmerBrokenCluster)
			hasBroken = true
		}
		goto st21
	tr48:
//line khmer_machine.rl:57
		te = p
		p--
		{
			foundSyllable(KhmerNonKhmerCluster)
		}
		goto st21
	st21:
//line NONE:1
		ts = 0

		if p++; p == pe {
			goto _test_eof21
		}
	st_case_21:
//line NONE:1
		ts = p

//line khmer_machine.go:219
		switch data[p] {
		case 4:
			goto st33
		case 15:
			goto tr29
		case 20:
			goto tr16
		case 21:
			goto tr25
		case 22:
			goto tr27
		case 23:
			goto tr23
		case 25:
			goto tr17
		case 26:
			goto tr18
		case 27:
			goto st36
		}
		switch {
		case data[p] < 5:
			if 1 <= data[p] && data[p] <= 2 {
				goto tr29
			}
		case data[p] > 6:
			if 10 <= data[p] && data[p] <= 11 {
				goto tr13
			}
		default:
			goto tr31
		}
		goto tr28
	tr29:
//line NONE:1
		te = p + 1

		goto st22
	st22:
		if p++; p == pe {
			goto _test_eof22
		}
	st_case_22:
//line khmer_machine.go:263
		switch data[p] {
		case 4:
			goto st23
		case 20:
			goto tr2
		case 21:
			goto tr10
		case 22:
			goto tr12
		case 23:
			goto tr8
		case 25:
			goto tr13
		case 26:
			goto tr4
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st10
		}
		goto tr32
	st23:
		if p++; p == pe {
			goto _test_eof23
		}
	st_case_23:
		if data[p] == 15 {
			goto tr35
		}
		if 1 <= data[p] && data[p] <= 2 {
			goto tr35
		}
		goto tr32
	tr35:
//line NONE:1
		te = p + 1

		goto st24
	st24:
		if p++; p == pe {
			goto _test_eof24
		}
	st_case_24:
//line khmer_machine.go:308
		switch data[p] {
		case 4:
			goto st23
		case 20:
			goto tr2
		case 21:
			goto tr10
		case 22:
			goto tr12
		case 23:
			goto tr8
		case 25:
			goto tr3
		case 26:
			goto tr4
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st0
		}
		goto tr32
	st0:
		if p++; p == pe {
			goto _test_eof0
		}
	st_case_0:
		switch data[p] {
		case 20:
			goto tr2
		case 25:
			goto tr3
		case 26:
			goto tr4
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr0
	st1:
		if p++; p == pe {
			goto _test_eof1
		}
	st_case_1:
		if data[p] == 26 {
			goto tr4
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr0
	tr4:
//line NONE:1
		te = p + 1

		goto st25
	st25:
		if p++; p == pe {
			goto _test_eof25
		}
	st_case_25:
//line khmer_machine.go:370
		switch data[p] {
		case 4:
			goto st2
		case 20:
			goto tr2
		case 21:
			goto tr10
		case 22:
			goto tr12
		case 23:
			goto tr8
		case 26:
			goto tr4
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st3
		}
		goto tr32
	st2:
		if p++; p == pe {
			goto _test_eof2
		}
	st_case_2:
		if data[p] == 15 {
			goto st26
		}
		if 1 <= data[p] && data[p] <= 2 {
			goto st26
		}
		goto tr0
	st26:
		if p++; p == pe {
			goto _test_eof26
		}
	st_case_26:
		if data[p] == 27 {
			goto st26
		}
		goto tr32
	st3:
		if p++; p == pe {
			goto _test_eof3
		}
	st_case_3:
		switch data[p] {
		case 20:
			goto tr2
		case 26:
			goto tr4
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr0
	tr2:
//line NONE:1
		te = p + 1

		goto st27
	st27:
		if p++; p == pe {
			goto _test_eof27
		}
	st_case_27:
//line khmer_machine.go:437
		switch data[p] {
		case 4:
			goto st2
		case 23:
			goto tr8
		case 26:
			goto tr2
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st4
		}
		goto tr32
	st4:
		if p++; p == pe {
			goto _test_eof4
		}
	st_case_4:
		if data[p] == 26 {
			goto tr2
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st4
		}
		goto tr0
	tr8:
//line NONE:1
		te = p + 1

		goto st28
	st28:
		if p++; p == pe {
			goto _test_eof28
		}
	st_case_28:
//line khmer_machine.go:474
		switch data[p] {
		case 4:
			goto st2
		case 26:
			goto tr8
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st5
		}
		goto tr32
	st5:
		if p++; p == pe {
			goto _test_eof5
		}
	st_case_5:
		if data[p] == 26 {
			goto tr8
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st5
		}
		goto tr0
	tr10:
//line NONE:1
		te = p + 1

		goto st29
	st29:
		if p++; p == pe {
			goto _test_eof29
		}
	st_case_29:
//line khmer_machine.go:509
		switch data[p] {
		case 4:
			goto st2
		case 20:
			goto tr2
		case 23:
			goto tr8
		case 26:
			goto tr10
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st6
		}
		goto tr32
	st6:
		if p++; p == pe {
			goto _test_eof6
		}
	st_case_6:
		switch data[p] {
		case 20:
			goto tr2
		case 26:
			goto tr10
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st7
		}
		goto tr0
	st7:
		if p++; p == pe {
			goto _test_eof7
		}
	st_case_7:
		if data[p] == 26 {
			goto tr10
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st7
		}
		goto tr0
	tr12:
//line NONE:1
		te = p + 1

		goto st30
	st30:
		if p++; p == pe {
			goto _test_eof30
		}
	st_case_30:
//line khmer_machine.go:563
		switch data[p] {
		case 4:
			goto st2
		case 20:
			goto tr2
		case 21:
			goto tr10
		case 23:
			goto tr8
		case 26:
			goto tr12
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st8
		}
		goto tr32
	st8:
		if p++; p == pe {
			goto _test_eof8
		}
	st_case_8:
		switch data[p] {
		case 20:
			goto tr2
		case 26:
			goto tr12
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr0
	st9:
		if p++; p == pe {
			goto _test_eof9
		}
	st_case_9:
		if data[p] == 26 {
			goto tr12
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr0
	tr3:
//line NONE:1
		te = p + 1

		goto st31
	st31:
		if p++; p == pe {
			goto _test_eof31
		}
	st_case_31:
//line khmer_machine.go:619
		switch data[p] {
		case 4:
			goto st23
		case 20:
			goto tr2
		case 21:
			goto tr10
		case 22:
			goto tr12
		case 23:
			goto tr8
		case 26:
			goto tr4
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st3
		}
		goto tr32
	st10:
		if p++; p == pe {
			goto _test_eof10
		}
	st_case_10:
		switch data[p] {
		case 20:
			goto tr2
		case 25:
			goto tr13
		case 26:
			goto tr4
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr0
	tr13:
//line NONE:1
		te = p + 1

		goto st32
	st32:
		if p++; p == pe {
			goto _test_eof32
		}
	st_case_32:
//line khmer_machine.go:667
		switch data[p] {
		case 4:
			goto st23
		case 20:
			goto tr2
		case 21:
			goto tr10
		case 22:
			goto tr12
		case 23:
			goto tr8
		case 25:
			goto tr3
		case 26:
			goto tr4
		case 27:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st3
		}
		goto tr32
	st33:
		if p++; p == pe {
			goto _test_eof33
		}
	st_case_33:
		if data[p] == 15 {
			goto tr42
		}
		if 1 <= data[p] && data[p] <= 2 {
			goto tr42
		}
		goto tr41
	tr42:
//line NONE:1
		te = p + 1

//line khmer_machine.rl:56
		act = 2
		goto st34
	st34:
		if p++; p == pe {
			goto _test_eof34
		}
	st_case_34:
//line khmer_machine.go:714
		switch data[p] {
		case 4:
			goto st33
		case 20:
			goto tr16
		case 21:
			goto tr25
		case 22:
			goto tr27
		case 23:
			goto tr23
		case 25:
			goto tr17
		case 26:
			goto tr18
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st11
		}
		goto tr41
	st11:
		if p++; p == pe {
			goto _test_eof11
		}
	st_case_11:
		switch data[p] {
		case 20:
			goto tr16
		case 25:
			goto tr17
		case 26:
			goto tr18
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st12
		}
		goto tr14
	st12:
		if p++; p == pe {
			goto _test_eof12
		}
	st_case_12:
		if data[p] == 26 {
			goto tr18
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st12
		}
		goto tr19
	tr18:
//line NONE:1
		te = p + 1

//line khmer_machine.rl:56
		act = 2
		goto st35
	st35:
		if p++; p == pe {
			goto _test_eof35
		}
	st_case_35:
//line khmer_machine.go:778
		switch data[p] {
		case 4:
			goto st13
		case 20:
			goto tr16
		case 21:
			goto tr25
		case 22:
			goto tr27
		case 23:
			goto tr23
		case 26:
			goto tr18
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st14
		}
		goto tr41
	st13:
		if p++; p == pe {
			goto _test_eof13
		}
	st_case_13:
		if data[p] == 15 {
			goto st36
		}
		if 1 <= data[p] && data[p] <= 2 {
			goto st36
		}
		goto tr14
	st36:
		if p++; p == pe {
			goto _test_eof36
		}
	st_case_36:
		if data[p] == 27 {
			goto st36
		}
		goto tr41
	st14:
		if p++; p == pe {
			goto _test_eof14
		}
	st_case_14:
		switch data[p] {
		case 20:
			goto tr16
		case 26:
			goto tr18
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st12
		}
		goto tr14
	tr16:
//line NONE:1
		te = p + 1

		goto st37
	st37:
		if p++; p == pe {
			goto _test_eof37
		}
	st_case_37:
//line khmer_machine.go:845
		switch data[p] {
		case 4:
			goto st13
		case 23:
			goto tr23
		case 26:
			goto tr16
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st15
		}
		goto tr41
	st15:
		if p++; p == pe {
			goto _test_eof15
		}
	st_case_15:
		if data[p] == 26 {
			goto tr16
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st15
		}
		goto tr14
	tr23:
//line NONE:1
		te = p + 1

		goto st38
	st38:
		if p++; p == pe {
			goto _test_eof38
		}
	st_case_38:
//line khmer_machine.go:882
		switch data[p] {
		case 4:
			goto st13
		case 26:
			goto tr23
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st16
		}
		goto tr41
	st16:
		if p++; p == pe {
			goto _test_eof16
		}
	st_case_16:
		if data[p] == 26 {
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st16
		}
		goto tr14
	tr25:
//line NONE:1
		te = p + 1

		goto st39
	st39:
		if p++; p == pe {
			goto _test_eof39
		}
	st_case_39:
//line khmer_machine.go:917
		switch data[p] {
		case 4:
			goto st13
		case 20:
			goto tr16
		case 23:
			goto tr23
		case 26:
			goto tr25
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr41
	st17:
		if p++; p == pe {
			goto _test_eof17
		}
	st_case_17:
		switch data[p] {
		case 20:
			goto tr16
		case 26:
			goto tr25
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st18
		}
		goto tr14
	st18:
		if p++; p == pe {
			goto _test_eof18
		}
	st_case_18:
		if data[p] == 26 {
			goto tr25
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st18
		}
		goto tr14
	tr27:
//line NONE:1
		te = p + 1

		goto st40
	st40:
		if p++; p == pe {
			goto _test_eof40
		}
	st_case_40:
//line khmer_machine.go:971
		switch data[p] {
		case 4:
			goto st13
		case 20:
			goto tr16
		case 21:
			goto tr25
		case 23:
			goto tr23
		case 26:
			goto tr27
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st19
		}
		goto tr41
	st19:
		if p++; p == pe {
			goto _test_eof19
		}
	st_case_19:
		switch data[p] {
		case 20:
			goto tr16
		case 26:
			goto tr27
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st20
		}
		goto tr14
	st20:
		if p++; p == pe {
			goto _test_eof20
		}
	st_case_20:
		if data[p] == 26 {
			goto tr27
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st20
		}
		goto tr14
	tr17:
//line NONE:1
		te = p + 1

//line khmer_machine.rl:56
		act = 2
		goto st41
	st41:
		if p++; p == pe {
			goto _test_eof41
		}
	st_case_41:
//line khmer_machine.go:1029
		switch data[p] {
		case 4:
			goto st33
		case 20:
			goto tr16
		case 21:
			goto tr25
		case 22:
			goto tr27
		case 23:
			goto tr23
		case 26:
			goto tr18
		case 27:
			goto st36
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st14
		}
		goto tr41
	tr31:
//line NONE:1
		te = p + 1

//line khmer_machine.rl:57
		act = 3
		goto st42
	st42:
		if p++; p == pe {
			goto _test_eof42
		}
	st_case_42:
//line khmer_machine.go:1062
		switch data[p] {
		case 20:
			goto tr16
		case 26:
			goto tr18
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st12
		}
		goto tr48
	st_out:
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
	_test_eof0:
		cs = 0
		goto _test_eof
	_test_eof1:
		cs = 1
		goto _test_eof
	_test_eof25:
		cs = 25
		goto _test_eof
	_test_eof2:
		cs = 2
		goto _test_eof
	_test_eof26:
		cs = 26
		goto _test_eof
	_test_eof3:
		cs = 3
		goto _test_eof
	_test_eof27:
		cs = 27
		goto _test_eof
	_test_eof4:
		cs = 4
		goto _test_eof
	_test_eof28:
		cs = 28
		goto _test_eof
	_test_eof5:
		cs = 5
		goto _test_eof
	_test_eof29:
		cs = 29
		goto _test_eof
	_test_eof6:
		cs = 6
		goto _test_eof
	_test_eof7:
		cs = 7
		goto _test_eof
	_test_eof30:
		cs = 30
		goto _test_eof
	_test_eof8:
		cs = 8
		goto _test_eof
	_test_eof9:
		cs = 9
		goto _test_eof
	_test_eof31:
		cs = 31
		goto _test_eof
	_test_eof10:
		cs = 10
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
	_test_eof11:
		cs = 11
		goto _test_eof
	_test_eof12:
		cs = 12
		goto _test_eof
	_test_eof35:
		cs = 35
		goto _test_eof
	_test_eof13:
		cs = 13
		goto _test_eof
	_test_eof36:
		cs = 36
		goto _test_eof
	_test_eof14:
		cs = 14
		goto _test_eof
	_test_eof37:
		cs = 37
		goto _test_eof
	_test_eof15:
		cs = 15
		goto _test_eof
	_test_eof38:
		cs = 38
		goto _test_eof
	_test_eof16:
		cs = 16
		goto _test_eof
	_test_eof39:
		cs = 39
		goto _test_eof
	_test_eof17:
		cs = 17
		goto _test_eof
	_test_eof18:
		cs = 18
		goto _test_eof
	_test_eof40:
		cs = 40
		goto _test_eof
	_test_eof19:
		cs = 19
		goto _test_eof
	_test_eof20:
		cs = 20
		goto _test_eof
	_test_eof41:
		cs = 41
		goto _test_eof
	_test_eof42:
		cs = 42
		goto _test_eof

	_test_eof:
		{
		}
		if p == eof {
			switch cs {
			case 22:
				goto tr32
			case 23:
				goto tr32
			case 24:
				goto tr32
			case 0:
				goto tr0
			case 1:
				goto tr0
			case 25:
				goto tr32
			case 2:
				goto tr0
			case 26:
				goto tr32
			case 3:
				goto tr0
			case 27:
				goto tr32
			case 4:
				goto tr0
			case 28:
				goto tr32
			case 5:
				goto tr0
			case 29:
				goto tr32
			case 6:
				goto tr0
			case 7:
				goto tr0
			case 30:
				goto tr32
			case 8:
				goto tr0
			case 9:
				goto tr0
			case 31:
				goto tr32
			case 10:
				goto tr0
			case 32:
				goto tr32
			case 33:
				goto tr41
			case 34:
				goto tr41
			case 11:
				goto tr14
			case 12:
				goto tr19
			case 35:
				goto tr41
			case 13:
				goto tr14
			case 36:
				goto tr41
			case 14:
				goto tr14
			case 37:
				goto tr41
			case 15:
				goto tr14
			case 38:
				goto tr41
			case 16:
				goto tr14
			case 39:
				goto tr41
			case 17:
				goto tr14
			case 18:
				goto tr14
			case 40:
				goto tr41
			case 19:
				goto tr14
			case 20:
				goto tr14
			case 41:
				goto tr41
			case 42:
				goto tr48
			}
		}

	}

//line khmer_machine.rl:99

	_ = cs            // Suppress unused variable warning
	_ = foundSyllable // May be unused if no syllables found

	return hasBroken
}
