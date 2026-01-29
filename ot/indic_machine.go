//line indic_machine.rl:1
// Copyright 2011,2012 Google, Inc.
// Ported to Go for textshape.
//
// HarfBuzz equivalent: hb-ot-shaper-indic-machine.rl

package ot

//line indic_machine.go:12
const indicSyllableMachine_start int = 31
const indicSyllableMachine_first_final int = 31
const indicSyllableMachine_error int = -1

const indicSyllableMachine_en_main int = 31

//line indic_machine.rl:12

//line indic_machine.rl:74

// FindSyllablesIndic finds Indic syllable boundaries in the buffer.
// It sets info[i].Syllable to (serial << 4 | type) for each glyph.
// HarfBuzz equivalent: find_syllables_indic() in hb-ot-shaper-indic-machine.hh
func FindSyllablesIndic(info []IndicInfo) (hasBroken bool) {
	if len(info) == 0 {
		return false
	}

	// Build category array
	data := make([]byte, len(info))
	for i := range info {
		data[i] = byte(info[i].Category)
	}

	var cs, p, pe, eof, ts, te, act int
	_ = act // Suppress unused variable warning

	pe = len(data)
	eof = pe

	var syllableSerial uint8 = 1

	foundSyllable := func(syllableType IndicSyllableType) {
		for i := ts; i < te; i++ {
			info[i].Syllable = (syllableSerial << 4) | uint8(syllableType)
		}
		syllableSerial++
		if syllableSerial == 16 {
			syllableSerial = 1
		}
	}

//line indic_machine.go:60
	{
		cs = indicSyllableMachine_start
		ts = 0
		te = 0
		act = 0
	}

//line indic_machine.go:68
	{
		if p == pe {
			goto _test_eof
		}
		switch cs {
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
		case 0:
			goto st_case_0
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
		case 1:
			goto st_case_1
		case 42:
			goto st_case_42
		case 2:
			goto st_case_2
		case 43:
			goto st_case_43
		case 44:
			goto st_case_44
		case 45:
			goto st_case_45
		case 3:
			goto st_case_3
		case 46:
			goto st_case_46
		case 4:
			goto st_case_4
		case 47:
			goto st_case_47
		case 48:
			goto st_case_48
		case 49:
			goto st_case_49
		case 5:
			goto st_case_5
		case 50:
			goto st_case_50
		case 6:
			goto st_case_6
		case 51:
			goto st_case_51
		case 52:
			goto st_case_52
		case 53:
			goto st_case_53
		case 54:
			goto st_case_54
		case 55:
			goto st_case_55
		case 56:
			goto st_case_56
		case 57:
			goto st_case_57
		case 58:
			goto st_case_58
		case 59:
			goto st_case_59
		case 7:
			goto st_case_7
		case 60:
			goto st_case_60
		case 8:
			goto st_case_8
		case 61:
			goto st_case_61
		case 62:
			goto st_case_62
		case 63:
			goto st_case_63
		case 64:
			goto st_case_64
		case 65:
			goto st_case_65
		case 9:
			goto st_case_9
		case 66:
			goto st_case_66
		case 67:
			goto st_case_67
		case 68:
			goto st_case_68
		case 10:
			goto st_case_10
		case 69:
			goto st_case_69
		case 11:
			goto st_case_11
		case 70:
			goto st_case_70
		case 71:
			goto st_case_71
		case 72:
			goto st_case_72
		case 73:
			goto st_case_73
		case 12:
			goto st_case_12
		case 74:
			goto st_case_74
		case 13:
			goto st_case_13
		case 75:
			goto st_case_75
		case 76:
			goto st_case_76
		case 77:
			goto st_case_77
		case 14:
			goto st_case_14
		case 78:
			goto st_case_78
		case 79:
			goto st_case_79
		case 80:
			goto st_case_80
		case 81:
			goto st_case_81
		case 82:
			goto st_case_82
		case 15:
			goto st_case_15
		case 83:
			goto st_case_83
		case 16:
			goto st_case_16
		case 84:
			goto st_case_84
		case 85:
			goto st_case_85
		case 86:
			goto st_case_86
		case 87:
			goto st_case_87
		case 88:
			goto st_case_88
		case 17:
			goto st_case_17
		case 89:
			goto st_case_89
		case 90:
			goto st_case_90
		case 91:
			goto st_case_91
		case 18:
			goto st_case_18
		case 92:
			goto st_case_92
		case 19:
			goto st_case_19
		case 93:
			goto st_case_93
		case 20:
			goto st_case_20
		case 94:
			goto st_case_94
		case 95:
			goto st_case_95
		case 96:
			goto st_case_96
		case 97:
			goto st_case_97
		case 21:
			goto st_case_21
		case 98:
			goto st_case_98
		case 99:
			goto st_case_99
		case 100:
			goto st_case_100
		case 101:
			goto st_case_101
		case 102:
			goto st_case_102
		case 103:
			goto st_case_103
		case 104:
			goto st_case_104
		case 105:
			goto st_case_105
		case 106:
			goto st_case_106
		case 22:
			goto st_case_22
		case 107:
			goto st_case_107
		case 23:
			goto st_case_23
		case 108:
			goto st_case_108
		case 109:
			goto st_case_109
		case 110:
			goto st_case_110
		case 111:
			goto st_case_111
		case 112:
			goto st_case_112
		case 24:
			goto st_case_24
		case 113:
			goto st_case_113
		case 114:
			goto st_case_114
		case 115:
			goto st_case_115
		case 25:
			goto st_case_25
		case 116:
			goto st_case_116
		case 26:
			goto st_case_26
		case 117:
			goto st_case_117
		case 27:
			goto st_case_27
		case 118:
			goto st_case_118
		case 119:
			goto st_case_119
		case 120:
			goto st_case_120
		case 121:
			goto st_case_121
		case 28:
			goto st_case_28
		case 122:
			goto st_case_122
		case 123:
			goto st_case_123
		case 124:
			goto st_case_124
		case 125:
			goto st_case_125
		case 126:
			goto st_case_126
		case 29:
			goto st_case_29
		case 127:
			goto st_case_127
		case 128:
			goto st_case_128
		case 129:
			goto st_case_129
		case 130:
			goto st_case_130
		case 131:
			goto st_case_131
		case 132:
			goto st_case_132
		case 133:
			goto st_case_133
		case 30:
			goto st_case_30
		case 134:
			goto st_case_134
		case 135:
			goto st_case_135
		case 136:
			goto st_case_136
		case 137:
			goto st_case_137
		}
		goto st_out
	tr0:
//line indic_machine.rl:65
		p = (te) - 1
		{
			foundSyllable(IndicConsonantSyllable)
		}
		goto st31
	tr9:
//line indic_machine.rl:66
		p = (te) - 1
		{
			foundSyllable(IndicVowelSyllable)
		}
		goto st31
	tr19:
//line indic_machine.rl:70
		p = (te) - 1
		{
			foundSyllable(IndicBrokenCluster)
			hasBroken = true
		}
		goto st31
	tr26:
//line NONE:1
		switch act {
		case 1:
			{
				p = (te) - 1
				foundSyllable(IndicConsonantSyllable)
			}
		case 5:
			{
				p = (te) - 1
				foundSyllable(IndicNonIndicCluster)
			}
		case 6:
			{
				p = (te) - 1
				foundSyllable(IndicBrokenCluster)
				hasBroken = true
			}
		case 7:
			{
				p = (te) - 1
				foundSyllable(IndicNonIndicCluster)
			}
		}

		goto st31
	tr29:
//line indic_machine.rl:67
		p = (te) - 1
		{
			foundSyllable(IndicStandaloneCluster)
		}
		goto st31
	tr39:
//line indic_machine.rl:68
		p = (te) - 1
		{
			foundSyllable(IndicSymbolCluster)
		}
		goto st31
	tr41:
//line indic_machine.rl:71
		te = p + 1
		{
			foundSyllable(IndicNonIndicCluster)
		}
		goto st31
	tr56:
//line indic_machine.rl:65
		te = p
		p--
		{
			foundSyllable(IndicConsonantSyllable)
		}
		goto st31
	tr76:
//line indic_machine.rl:66
		te = p
		p--
		{
			foundSyllable(IndicVowelSyllable)
		}
		goto st31
	tr101:
//line indic_machine.rl:70
		te = p
		p--
		{
			foundSyllable(IndicBrokenCluster)
			hasBroken = true
		}
		goto st31
	tr118:
//line indic_machine.rl:71
		te = p
		p--
		{
			foundSyllable(IndicNonIndicCluster)
		}
		goto st31
	tr119:
//line indic_machine.rl:67
		te = p
		p--
		{
			foundSyllable(IndicStandaloneCluster)
		}
		goto st31
	tr146:
//line indic_machine.rl:68
		te = p
		p--
		{
			foundSyllable(IndicSymbolCluster)
		}
		goto st31
	st31:
//line NONE:1
		ts = 0

		if p++; p == pe {
			goto _test_eof31
		}
	st_case_31:
//line NONE:1
		ts = p

//line indic_machine.go:447
		switch data[p] {
		case 1:
			goto tr42
		case 2:
			goto tr43
		case 3:
			goto tr44
		case 4:
			goto st81
		case 5:
			goto tr46
		case 6:
			goto tr47
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 14:
			goto tr50
		case 15:
			goto tr51
		case 16:
			goto tr52
		case 17:
			goto tr53
		case 18:
			goto st137
		case 57:
			goto tr55
		}
		if 10 <= data[p] && data[p] <= 11 {
			goto tr49
		}
		goto tr41
	tr42:
//line NONE:1
		te = p + 1

		goto st32
	st32:
		if p++; p == pe {
			goto _test_eof32
		}
	st_case_32:
//line indic_machine.go:498
		switch data[p] {
		case 3:
			goto tr57
		case 4:
			goto st35
		case 5:
			goto st6
		case 6:
			goto tr60
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 12:
			goto tr8
		case 13:
			goto tr4
		case 16:
			goto tr62
		case 57:
			goto st46
		}
		goto tr56
	tr57:
//line NONE:1
		te = p + 1

		goto st33
	st33:
		if p++; p == pe {
			goto _test_eof33
		}
	st_case_33:
//line indic_machine.go:534
		switch data[p] {
		case 3:
			goto tr63
		case 4:
			goto st35
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 16:
			goto tr62
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr56
	tr63:
//line NONE:1
		te = p + 1

		goto st34
	st34:
		if p++; p == pe {
			goto _test_eof34
		}
	st_case_34:
//line indic_machine.go:567
		switch data[p] {
		case 4:
			goto st35
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 16:
			goto tr62
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr56
	st35:
		if p++; p == pe {
			goto _test_eof35
		}
	st_case_35:
		switch data[p] {
		case 1:
			goto tr42
		case 5:
			goto tr65
		case 6:
			goto tr66
		case 8:
			goto st37
		case 9:
			goto st38
		case 15:
			goto tr42
		case 57:
			goto st37
		}
		goto tr56
	tr65:
//line NONE:1
		te = p + 1

		goto st36
	st36:
		if p++; p == pe {
			goto _test_eof36
		}
	st_case_36:
//line indic_machine.go:620
		switch data[p] {
		case 8:
			goto st37
		case 9:
			goto st38
		case 57:
			goto st37
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st0
		}
		goto tr56
	st0:
		if p++; p == pe {
			goto _test_eof0
		}
	st_case_0:
		switch data[p] {
		case 8:
			goto st37
		case 57:
			goto st37
		}
		goto tr0
	st37:
		if p++; p == pe {
			goto _test_eof37
		}
	st_case_37:
		switch data[p] {
		case 5:
			goto st38
		case 8:
			goto st39
		case 9:
			goto st38
		case 57:
			goto st39
		}
		goto tr56
	st38:
		if p++; p == pe {
			goto _test_eof38
		}
	st_case_38:
		if data[p] == 9 {
			goto st38
		}
		goto tr56
	st39:
		if p++; p == pe {
			goto _test_eof39
		}
	st_case_39:
		switch data[p] {
		case 5:
			goto st38
		case 9:
			goto st38
		}
		goto tr56
	tr66:
//line NONE:1
		te = p + 1

		goto st40
	st40:
		if p++; p == pe {
			goto _test_eof40
		}
	st_case_40:
//line indic_machine.go:692
		switch data[p] {
		case 1:
			goto tr42
		case 3:
			goto tr69
		case 8:
			goto st37
		case 9:
			goto st38
		case 15:
			goto tr42
		case 57:
			goto st37
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st0
		}
		goto tr56
	tr69:
//line NONE:1
		te = p + 1

		goto st41
	st41:
		if p++; p == pe {
			goto _test_eof41
		}
	st_case_41:
//line indic_machine.go:721
		switch data[p] {
		case 1:
			goto tr42
		case 8:
			goto st37
		case 9:
			goto st38
		case 15:
			goto tr42
		case 57:
			goto st37
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st0
		}
		goto tr56
	st1:
		if p++; p == pe {
			goto _test_eof1
		}
	st_case_1:
		switch data[p] {
		case 4:
			goto tr2
		case 7:
			goto tr4
		case 8:
			goto st46
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st2
		}
		goto tr0
	tr2:
//line NONE:1
		te = p + 1

		goto st42
	st42:
		if p++; p == pe {
			goto _test_eof42
		}
	st_case_42:
//line indic_machine.go:769
		switch data[p] {
		case 1:
			goto tr42
		case 5:
			goto st0
		case 6:
			goto tr66
		case 8:
			goto st37
		case 9:
			goto st38
		case 15:
			goto tr42
		case 57:
			goto st37
		}
		goto tr56
	st2:
		if p++; p == pe {
			goto _test_eof2
		}
	st_case_2:
		switch data[p] {
		case 7:
			goto tr4
		case 8:
			goto st4
		case 13:
			goto tr4
		case 57:
			goto st4
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st2
		}
		goto tr0
	tr4:
//line NONE:1
		te = p + 1

		goto st43
	st43:
		if p++; p == pe {
			goto _test_eof43
		}
	st_case_43:
//line indic_machine.go:816
		switch data[p] {
		case 3:
			goto tr70
		case 4:
			goto tr71
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st3
		}
		goto tr56
	tr70:
//line NONE:1
		te = p + 1

		goto st44
	st44:
		if p++; p == pe {
			goto _test_eof44
		}
	st_case_44:
//line indic_machine.go:847
		switch data[p] {
		case 4:
			goto tr71
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st3
		}
		goto tr56
	tr71:
//line NONE:1
		te = p + 1

		goto st45
	st45:
		if p++; p == pe {
			goto _test_eof45
		}
	st_case_45:
//line indic_machine.go:876
		switch data[p] {
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st3
		}
		goto tr56
	st3:
		if p++; p == pe {
			goto _test_eof3
		}
	st_case_3:
		switch data[p] {
		case 7:
			goto tr4
		case 8:
			goto st46
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st2
		}
		goto tr0
	st46:
		if p++; p == pe {
			goto _test_eof46
		}
	st_case_46:
		switch data[p] {
		case 5:
			goto st38
		case 8:
			goto st39
		case 9:
			goto st38
		case 13:
			goto tr4
		case 57:
			goto st39
		}
		goto tr56
	st4:
		if p++; p == pe {
			goto _test_eof4
		}
	st_case_4:
		if data[p] == 13 {
			goto tr4
		}
		goto tr0
	tr62:
//line NONE:1
		te = p + 1

		goto st47
	st47:
		if p++; p == pe {
			goto _test_eof47
		}
	st_case_47:
//line indic_machine.go:949
		switch data[p] {
		case 4:
			goto st48
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st5
		}
		goto tr56
	st48:
		if p++; p == pe {
			goto _test_eof48
		}
	st_case_48:
		switch data[p] {
		case 5:
			goto tr65
		case 6:
			goto tr75
		case 8:
			goto st37
		case 9:
			goto st38
		case 57:
			goto st37
		}
		goto tr56
	tr75:
//line NONE:1
		te = p + 1

		goto st49
	st49:
		if p++; p == pe {
			goto _test_eof49
		}
	st_case_49:
//line indic_machine.go:996
		switch data[p] {
		case 3:
			goto tr65
		case 8:
			goto st37
		case 9:
			goto st38
		case 57:
			goto st37
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st0
		}
		goto tr56
	st5:
		if p++; p == pe {
			goto _test_eof5
		}
	st_case_5:
		switch data[p] {
		case 4:
			goto tr7
		case 7:
			goto tr4
		case 8:
			goto st46
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st2
		}
		goto tr0
	tr7:
//line NONE:1
		te = p + 1

		goto st50
	st50:
		if p++; p == pe {
			goto _test_eof50
		}
	st_case_50:
//line indic_machine.go:1042
		switch data[p] {
		case 5:
			goto st0
		case 6:
			goto tr75
		case 8:
			goto st37
		case 9:
			goto st38
		case 57:
			goto st37
		}
		goto tr56
	st6:
		if p++; p == pe {
			goto _test_eof6
		}
	st_case_6:
		switch data[p] {
		case 4:
			goto tr2
		case 7:
			goto tr4
		case 8:
			goto st46
		case 12:
			goto tr8
		case 13:
			goto tr4
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st2
		}
		goto tr0
	tr8:
//line NONE:1
		te = p + 1

		goto st51
	st51:
		if p++; p == pe {
			goto _test_eof51
		}
	st_case_51:
//line indic_machine.go:1089
		switch data[p] {
		case 3:
			goto tr57
		case 4:
			goto st35
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 13:
			goto tr4
		case 16:
			goto tr62
		case 57:
			goto st46
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st1
		}
		goto tr56
	tr60:
//line NONE:1
		te = p + 1

		goto st52
	st52:
		if p++; p == pe {
			goto _test_eof52
		}
	st_case_52:
//line indic_machine.go:1122
		switch data[p] {
		case 3:
			goto tr57
		case 4:
			goto st35
		case 5:
			goto st6
		case 6:
			goto st1
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 12:
			goto tr8
		case 13:
			goto tr4
		case 16:
			goto tr62
		case 57:
			goto st46
		}
		goto tr56
	tr43:
//line NONE:1
		te = p + 1

		goto st53
	st53:
		if p++; p == pe {
			goto _test_eof53
		}
	st_case_53:
//line indic_machine.go:1158
		switch data[p] {
		case 3:
			goto tr77
		case 4:
			goto st56
		case 5:
			goto st14
		case 6:
			goto tr80
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 12:
			goto tr18
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		goto tr76
	tr77:
//line NONE:1
		te = p + 1

		goto st54
	st54:
		if p++; p == pe {
			goto _test_eof54
		}
	st_case_54:
//line indic_machine.go:1194
		switch data[p] {
		case 3:
			goto tr83
		case 4:
			goto st56
		case 5:
			goto st7
		case 6:
			goto tr80
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		goto tr76
	tr83:
//line NONE:1
		te = p + 1

		goto st55
	st55:
		if p++; p == pe {
			goto _test_eof55
		}
	st_case_55:
//line indic_machine.go:1228
		switch data[p] {
		case 4:
			goto st56
		case 5:
			goto st7
		case 6:
			goto tr80
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		goto tr76
	st56:
		if p++; p == pe {
			goto _test_eof56
		}
	st_case_56:
		switch data[p] {
		case 1:
			goto tr85
		case 5:
			goto tr86
		case 6:
			goto tr87
		case 8:
			goto st61
		case 9:
			goto st62
		case 15:
			goto tr85
		case 57:
			goto st61
		}
		goto tr76
	tr85:
//line NONE:1
		te = p + 1

		goto st57
	st57:
		if p++; p == pe {
			goto _test_eof57
		}
	st_case_57:
//line indic_machine.go:1282
		switch data[p] {
		case 3:
			goto tr88
		case 4:
			goto st56
		case 5:
			goto st13
		case 6:
			goto tr90
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 12:
			goto tr17
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		goto tr76
	tr88:
//line NONE:1
		te = p + 1

		goto st58
	st58:
		if p++; p == pe {
			goto _test_eof58
		}
	st_case_58:
//line indic_machine.go:1318
		switch data[p] {
		case 3:
			goto tr91
		case 4:
			goto st56
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st7
		}
		goto tr76
	tr91:
//line NONE:1
		te = p + 1

		goto st59
	st59:
		if p++; p == pe {
			goto _test_eof59
		}
	st_case_59:
//line indic_machine.go:1351
		switch data[p] {
		case 4:
			goto st56
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st7
		}
		goto tr76
	st7:
		if p++; p == pe {
			goto _test_eof7
		}
	st_case_7:
		switch data[p] {
		case 4:
			goto tr10
		case 7:
			goto tr12
		case 8:
			goto st69
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr9
	tr10:
//line NONE:1
		te = p + 1

		goto st60
	st60:
		if p++; p == pe {
			goto _test_eof60
		}
	st_case_60:
//line indic_machine.go:1403
		switch data[p] {
		case 1:
			goto tr85
		case 5:
			goto st8
		case 6:
			goto tr87
		case 8:
			goto st61
		case 9:
			goto st62
		case 15:
			goto tr85
		case 57:
			goto st61
		}
		goto tr76
	st8:
		if p++; p == pe {
			goto _test_eof8
		}
	st_case_8:
		switch data[p] {
		case 8:
			goto st61
		case 57:
			goto st61
		}
		goto tr9
	st61:
		if p++; p == pe {
			goto _test_eof61
		}
	st_case_61:
		switch data[p] {
		case 5:
			goto st62
		case 8:
			goto st63
		case 9:
			goto st62
		case 57:
			goto st63
		}
		goto tr76
	st62:
		if p++; p == pe {
			goto _test_eof62
		}
	st_case_62:
		if data[p] == 9 {
			goto st62
		}
		goto tr76
	st63:
		if p++; p == pe {
			goto _test_eof63
		}
	st_case_63:
		switch data[p] {
		case 5:
			goto st62
		case 9:
			goto st62
		}
		goto tr76
	tr87:
//line NONE:1
		te = p + 1

		goto st64
	st64:
		if p++; p == pe {
			goto _test_eof64
		}
	st_case_64:
//line indic_machine.go:1480
		switch data[p] {
		case 1:
			goto tr85
		case 3:
			goto tr94
		case 8:
			goto st61
		case 9:
			goto st62
		case 15:
			goto tr85
		case 57:
			goto st61
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st8
		}
		goto tr76
	tr94:
//line NONE:1
		te = p + 1

		goto st65
	st65:
		if p++; p == pe {
			goto _test_eof65
		}
	st_case_65:
//line indic_machine.go:1509
		switch data[p] {
		case 1:
			goto tr85
		case 8:
			goto st61
		case 9:
			goto st62
		case 15:
			goto tr85
		case 57:
			goto st61
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st8
		}
		goto tr76
	st9:
		if p++; p == pe {
			goto _test_eof9
		}
	st_case_9:
		switch data[p] {
		case 7:
			goto tr12
		case 8:
			goto st11
		case 13:
			goto tr12
		case 57:
			goto st11
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr9
	tr12:
//line NONE:1
		te = p + 1

		goto st66
	st66:
		if p++; p == pe {
			goto _test_eof66
		}
	st_case_66:
//line indic_machine.go:1555
		switch data[p] {
		case 3:
			goto tr95
		case 4:
			goto tr96
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st10
		}
		goto tr76
	tr95:
//line NONE:1
		te = p + 1

		goto st67
	st67:
		if p++; p == pe {
			goto _test_eof67
		}
	st_case_67:
//line indic_machine.go:1586
		switch data[p] {
		case 4:
			goto tr96
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st10
		}
		goto tr76
	tr96:
//line NONE:1
		te = p + 1

		goto st68
	st68:
		if p++; p == pe {
			goto _test_eof68
		}
	st_case_68:
//line indic_machine.go:1615
		switch data[p] {
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st10
		}
		goto tr76
	st10:
		if p++; p == pe {
			goto _test_eof10
		}
	st_case_10:
		switch data[p] {
		case 7:
			goto tr12
		case 8:
			goto st69
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr9
	st69:
		if p++; p == pe {
			goto _test_eof69
		}
	st_case_69:
		switch data[p] {
		case 5:
			goto st62
		case 8:
			goto st63
		case 9:
			goto st62
		case 13:
			goto tr12
		case 57:
			goto st63
		}
		goto tr76
	st11:
		if p++; p == pe {
			goto _test_eof11
		}
	st_case_11:
		if data[p] == 13 {
			goto tr12
		}
		goto tr9
	tr82:
//line NONE:1
		te = p + 1

		goto st70
	st70:
		if p++; p == pe {
			goto _test_eof70
		}
	st_case_70:
//line indic_machine.go:1688
		switch data[p] {
		case 4:
			goto st71
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st12
		}
		goto tr76
	st71:
		if p++; p == pe {
			goto _test_eof71
		}
	st_case_71:
		switch data[p] {
		case 5:
			goto tr86
		case 6:
			goto tr100
		case 8:
			goto st61
		case 9:
			goto st62
		case 57:
			goto st61
		}
		goto tr76
	tr86:
//line NONE:1
		te = p + 1

		goto st72
	st72:
		if p++; p == pe {
			goto _test_eof72
		}
	st_case_72:
//line indic_machine.go:1735
		switch data[p] {
		case 8:
			goto st61
		case 9:
			goto st62
		case 57:
			goto st61
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st8
		}
		goto tr76
	tr100:
//line NONE:1
		te = p + 1

		goto st73
	st73:
		if p++; p == pe {
			goto _test_eof73
		}
	st_case_73:
//line indic_machine.go:1758
		switch data[p] {
		case 3:
			goto tr86
		case 8:
			goto st61
		case 9:
			goto st62
		case 57:
			goto st61
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st8
		}
		goto tr76
	st12:
		if p++; p == pe {
			goto _test_eof12
		}
	st_case_12:
		switch data[p] {
		case 4:
			goto tr16
		case 7:
			goto tr12
		case 8:
			goto st69
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr9
	tr16:
//line NONE:1
		te = p + 1

		goto st74
	st74:
		if p++; p == pe {
			goto _test_eof74
		}
	st_case_74:
//line indic_machine.go:1804
		switch data[p] {
		case 5:
			goto st8
		case 6:
			goto tr100
		case 8:
			goto st61
		case 9:
			goto st62
		case 57:
			goto st61
		}
		goto tr76
	st13:
		if p++; p == pe {
			goto _test_eof13
		}
	st_case_13:
		switch data[p] {
		case 4:
			goto tr10
		case 7:
			goto tr12
		case 8:
			goto st69
		case 12:
			goto tr17
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr9
	tr17:
//line NONE:1
		te = p + 1

		goto st75
	st75:
		if p++; p == pe {
			goto _test_eof75
		}
	st_case_75:
//line indic_machine.go:1851
		switch data[p] {
		case 3:
			goto tr88
		case 4:
			goto st56
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st7
		}
		goto tr76
	tr90:
//line NONE:1
		te = p + 1

		goto st76
	st76:
		if p++; p == pe {
			goto _test_eof76
		}
	st_case_76:
//line indic_machine.go:1884
		switch data[p] {
		case 3:
			goto tr88
		case 4:
			goto st56
		case 5:
			goto st13
		case 6:
			goto st7
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 12:
			goto tr17
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		goto tr76
	tr80:
//line NONE:1
		te = p + 1

		goto st77
	st77:
		if p++; p == pe {
			goto _test_eof77
		}
	st_case_77:
//line indic_machine.go:1920
		switch data[p] {
		case 4:
			goto tr10
		case 7:
			goto tr12
		case 8:
			goto st69
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr76
	st14:
		if p++; p == pe {
			goto _test_eof14
		}
	st_case_14:
		switch data[p] {
		case 4:
			goto tr10
		case 7:
			goto tr12
		case 8:
			goto st69
		case 12:
			goto tr18
		case 13:
			goto tr12
		case 57:
			goto st69
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st9
		}
		goto tr9
	tr18:
//line NONE:1
		te = p + 1

		goto st78
	st78:
		if p++; p == pe {
			goto _test_eof78
		}
	st_case_78:
//line indic_machine.go:1970
		switch data[p] {
		case 3:
			goto tr77
		case 4:
			goto st56
		case 5:
			goto st7
		case 6:
			goto tr80
		case 7:
			goto tr12
		case 8:
			goto st69
		case 9:
			goto st62
		case 13:
			goto tr12
		case 16:
			goto tr82
		case 57:
			goto st69
		}
		goto tr76
	tr44:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st79
	st79:
		if p++; p == pe {
			goto _test_eof79
		}
	st_case_79:
//line indic_machine.go:2006
		switch data[p] {
		case 3:
			goto tr102
		case 4:
			goto st81
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 16:
			goto tr52
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st20
		}
		goto tr101
	tr102:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st80
	st80:
		if p++; p == pe {
			goto _test_eof80
		}
	st_case_80:
//line indic_machine.go:2041
		switch data[p] {
		case 4:
			goto st81
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 16:
			goto tr52
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st20
		}
		goto tr101
	st81:
		if p++; p == pe {
			goto _test_eof81
		}
	st_case_81:
		switch data[p] {
		case 1:
			goto tr104
		case 5:
			goto tr105
		case 6:
			goto tr106
		case 8:
			goto st84
		case 9:
			goto st85
		case 15:
			goto tr104
		case 57:
			goto st84
		}
		goto tr101
	tr104:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st82
	st82:
		if p++; p == pe {
			goto _test_eof82
		}
	st_case_82:
//line indic_machine.go:2096
		switch data[p] {
		case 3:
			goto tr44
		case 4:
			goto st81
		case 5:
			goto st15
		case 6:
			goto tr108
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 16:
			goto tr52
		case 57:
			goto tr23
		}
		goto tr101
	st15:
		if p++; p == pe {
			goto _test_eof15
		}
	st_case_15:
		switch data[p] {
		case 4:
			goto tr20
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr19
	tr20:
//line NONE:1
		te = p + 1

		goto st83
	st83:
		if p++; p == pe {
			goto _test_eof83
		}
	st_case_83:
//line indic_machine.go:2155
		switch data[p] {
		case 1:
			goto tr104
		case 5:
			goto st16
		case 6:
			goto tr106
		case 8:
			goto st84
		case 9:
			goto st85
		case 15:
			goto tr104
		case 57:
			goto st84
		}
		goto tr101
	st16:
		if p++; p == pe {
			goto _test_eof16
		}
	st_case_16:
		switch data[p] {
		case 8:
			goto st84
		case 57:
			goto st84
		}
		goto tr19
	st84:
		if p++; p == pe {
			goto _test_eof84
		}
	st_case_84:
		switch data[p] {
		case 5:
			goto st85
		case 8:
			goto st86
		case 9:
			goto st85
		case 57:
			goto st86
		}
		goto tr101
	st85:
		if p++; p == pe {
			goto _test_eof85
		}
	st_case_85:
		if data[p] == 9 {
			goto st85
		}
		goto tr101
	st86:
		if p++; p == pe {
			goto _test_eof86
		}
	st_case_86:
		switch data[p] {
		case 5:
			goto st85
		case 9:
			goto st85
		}
		goto tr101
	tr106:
//line NONE:1
		te = p + 1

		goto st87
	st87:
		if p++; p == pe {
			goto _test_eof87
		}
	st_case_87:
//line indic_machine.go:2232
		switch data[p] {
		case 1:
			goto tr104
		case 3:
			goto tr111
		case 8:
			goto st84
		case 9:
			goto st85
		case 15:
			goto tr104
		case 57:
			goto st84
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st16
		}
		goto tr101
	tr111:
//line NONE:1
		te = p + 1

		goto st88
	st88:
		if p++; p == pe {
			goto _test_eof88
		}
	st_case_88:
//line indic_machine.go:2261
		switch data[p] {
		case 1:
			goto tr104
		case 8:
			goto st84
		case 9:
			goto st85
		case 15:
			goto tr104
		case 57:
			goto st84
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st16
		}
		goto tr101
	st17:
		if p++; p == pe {
			goto _test_eof17
		}
	st_case_17:
		switch data[p] {
		case 7:
			goto tr22
		case 8:
			goto st19
		case 13:
			goto tr22
		case 57:
			goto st19
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr26
	tr22:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st89
	st89:
		if p++; p == pe {
			goto _test_eof89
		}
	st_case_89:
//line indic_machine.go:2309
		switch data[p] {
		case 3:
			goto tr112
		case 4:
			goto tr113
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st18
		}
		goto tr101
	tr112:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st90
	st90:
		if p++; p == pe {
			goto _test_eof90
		}
	st_case_90:
//line indic_machine.go:2342
		switch data[p] {
		case 4:
			goto tr113
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st18
		}
		goto tr101
	tr113:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st91
	st91:
		if p++; p == pe {
			goto _test_eof91
		}
	st_case_91:
//line indic_machine.go:2373
		switch data[p] {
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st18
		}
		goto tr101
	st18:
		if p++; p == pe {
			goto _test_eof18
		}
	st_case_18:
		switch data[p] {
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr19
	tr23:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st92
	tr55:
//line NONE:1
		te = p + 1

//line indic_machine.rl:69
		act = 5
		goto st92
	st92:
		if p++; p == pe {
			goto _test_eof92
		}
	st_case_92:
//line indic_machine.go:2428
		switch data[p] {
		case 5:
			goto st85
		case 8:
			goto st86
		case 9:
			goto st85
		case 13:
			goto tr22
		case 57:
			goto st86
		}
		goto tr26
	st19:
		if p++; p == pe {
			goto _test_eof19
		}
	st_case_19:
		if data[p] == 13 {
			goto tr22
		}
		goto tr26
	tr24:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st93
	st93:
		if p++; p == pe {
			goto _test_eof93
		}
	st_case_93:
//line indic_machine.go:2463
		switch data[p] {
		case 3:
			goto tr44
		case 4:
			goto st81
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 16:
			goto tr52
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st20
		}
		goto tr101
	st20:
		if p++; p == pe {
			goto _test_eof20
		}
	st_case_20:
		switch data[p] {
		case 4:
			goto tr20
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr19
	tr52:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st94
	st94:
		if p++; p == pe {
			goto _test_eof94
		}
	st_case_94:
//line indic_machine.go:2519
		switch data[p] {
		case 4:
			goto st95
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st21
		}
		goto tr101
	st95:
		if p++; p == pe {
			goto _test_eof95
		}
	st_case_95:
		switch data[p] {
		case 5:
			goto tr105
		case 6:
			goto tr117
		case 8:
			goto st84
		case 9:
			goto st85
		case 57:
			goto st84
		}
		goto tr101
	tr105:
//line NONE:1
		te = p + 1

		goto st96
	st96:
		if p++; p == pe {
			goto _test_eof96
		}
	st_case_96:
//line indic_machine.go:2566
		switch data[p] {
		case 8:
			goto st84
		case 9:
			goto st85
		case 57:
			goto st84
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st16
		}
		goto tr101
	tr117:
//line NONE:1
		te = p + 1

		goto st97
	st97:
		if p++; p == pe {
			goto _test_eof97
		}
	st_case_97:
//line indic_machine.go:2589
		switch data[p] {
		case 3:
			goto tr105
		case 8:
			goto st84
		case 9:
			goto st85
		case 57:
			goto st84
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st16
		}
		goto tr101
	st21:
		if p++; p == pe {
			goto _test_eof21
		}
	st_case_21:
		switch data[p] {
		case 4:
			goto tr28
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr19
	tr28:
//line NONE:1
		te = p + 1

		goto st98
	st98:
		if p++; p == pe {
			goto _test_eof98
		}
	st_case_98:
//line indic_machine.go:2635
		switch data[p] {
		case 5:
			goto st16
		case 6:
			goto tr117
		case 8:
			goto st84
		case 9:
			goto st85
		case 57:
			goto st84
		}
		goto tr101
	tr108:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st99
	st99:
		if p++; p == pe {
			goto _test_eof99
		}
	st_case_99:
//line indic_machine.go:2661
		switch data[p] {
		case 3:
			goto tr44
		case 4:
			goto st81
		case 5:
			goto st15
		case 6:
			goto st20
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 16:
			goto tr52
		case 57:
			goto tr23
		}
		goto tr101
	tr46:
//line NONE:1
		te = p + 1

//line indic_machine.rl:71
		act = 7
		goto st100
	st100:
		if p++; p == pe {
			goto _test_eof100
		}
	st_case_100:
//line indic_machine.go:2699
		switch data[p] {
		case 4:
			goto tr20
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr118
	tr47:
//line NONE:1
		te = p + 1

//line indic_machine.rl:71
		act = 7
		goto st101
	st101:
		if p++; p == pe {
			goto _test_eof101
		}
	st_case_101:
//line indic_machine.go:2730
		switch data[p] {
		case 4:
			goto tr20
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 13:
			goto tr22
		case 57:
			goto tr23
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr118
	tr49:
//line NONE:1
		te = p + 1

		goto st102
	st102:
		if p++; p == pe {
			goto _test_eof102
		}
	st_case_102:
//line indic_machine.go:2757
		switch data[p] {
		case 3:
			goto tr120
		case 4:
			goto st105
		case 5:
			goto st22
		case 6:
			goto st27
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 12:
			goto tr34
		case 13:
			goto tr32
		case 16:
			goto tr125
		case 57:
			goto st116
		}
		goto tr119
	tr120:
//line NONE:1
		te = p + 1

		goto st103
	st103:
		if p++; p == pe {
			goto _test_eof103
		}
	st_case_103:
//line indic_machine.go:2793
		switch data[p] {
		case 3:
			goto tr126
		case 4:
			goto st105
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 16:
			goto tr125
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st27
		}
		goto tr119
	tr126:
//line NONE:1
		te = p + 1

		goto st104
	st104:
		if p++; p == pe {
			goto _test_eof104
		}
	st_case_104:
//line indic_machine.go:2826
		switch data[p] {
		case 4:
			goto st105
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 16:
			goto tr125
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st27
		}
		goto tr119
	st105:
		if p++; p == pe {
			goto _test_eof105
		}
	st_case_105:
		switch data[p] {
		case 1:
			goto tr127
		case 5:
			goto tr128
		case 6:
			goto tr129
		case 8:
			goto st108
		case 9:
			goto st109
		case 15:
			goto tr127
		case 57:
			goto st108
		}
		goto tr119
	tr127:
//line NONE:1
		te = p + 1

		goto st106
	st106:
		if p++; p == pe {
			goto _test_eof106
		}
	st_case_106:
//line indic_machine.go:2879
		switch data[p] {
		case 3:
			goto tr120
		case 4:
			goto st105
		case 5:
			goto st22
		case 6:
			goto tr49
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 12:
			goto tr34
		case 13:
			goto tr32
		case 16:
			goto tr125
		case 57:
			goto st116
		}
		goto tr119
	st22:
		if p++; p == pe {
			goto _test_eof22
		}
	st_case_22:
		switch data[p] {
		case 4:
			goto tr30
		case 7:
			goto tr32
		case 8:
			goto st116
		case 12:
			goto tr34
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st24
		}
		goto tr29
	tr30:
//line NONE:1
		te = p + 1

		goto st107
	st107:
		if p++; p == pe {
			goto _test_eof107
		}
	st_case_107:
//line indic_machine.go:2938
		switch data[p] {
		case 1:
			goto tr127
		case 5:
			goto st23
		case 6:
			goto tr129
		case 8:
			goto st108
		case 9:
			goto st109
		case 15:
			goto tr127
		case 57:
			goto st108
		}
		goto tr119
	st23:
		if p++; p == pe {
			goto _test_eof23
		}
	st_case_23:
		switch data[p] {
		case 8:
			goto st108
		case 57:
			goto st108
		}
		goto tr29
	st108:
		if p++; p == pe {
			goto _test_eof108
		}
	st_case_108:
		switch data[p] {
		case 5:
			goto st109
		case 8:
			goto st110
		case 9:
			goto st109
		case 57:
			goto st110
		}
		goto tr119
	st109:
		if p++; p == pe {
			goto _test_eof109
		}
	st_case_109:
		if data[p] == 9 {
			goto st109
		}
		goto tr119
	st110:
		if p++; p == pe {
			goto _test_eof110
		}
	st_case_110:
		switch data[p] {
		case 5:
			goto st109
		case 9:
			goto st109
		}
		goto tr119
	tr129:
//line NONE:1
		te = p + 1

		goto st111
	st111:
		if p++; p == pe {
			goto _test_eof111
		}
	st_case_111:
//line indic_machine.go:3015
		switch data[p] {
		case 1:
			goto tr127
		case 3:
			goto tr132
		case 8:
			goto st108
		case 9:
			goto st109
		case 15:
			goto tr127
		case 57:
			goto st108
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st23
		}
		goto tr119
	tr132:
//line NONE:1
		te = p + 1

		goto st112
	st112:
		if p++; p == pe {
			goto _test_eof112
		}
	st_case_112:
//line indic_machine.go:3044
		switch data[p] {
		case 1:
			goto tr127
		case 8:
			goto st108
		case 9:
			goto st109
		case 15:
			goto tr127
		case 57:
			goto st108
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st23
		}
		goto tr119
	st24:
		if p++; p == pe {
			goto _test_eof24
		}
	st_case_24:
		switch data[p] {
		case 7:
			goto tr32
		case 8:
			goto st26
		case 13:
			goto tr32
		case 57:
			goto st26
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st24
		}
		goto tr29
	tr32:
//line NONE:1
		te = p + 1

		goto st113
	st113:
		if p++; p == pe {
			goto _test_eof113
		}
	st_case_113:
//line indic_machine.go:3090
		switch data[p] {
		case 3:
			goto tr133
		case 4:
			goto tr134
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st25
		}
		goto tr119
	tr133:
//line NONE:1
		te = p + 1

		goto st114
	st114:
		if p++; p == pe {
			goto _test_eof114
		}
	st_case_114:
//line indic_machine.go:3121
		switch data[p] {
		case 4:
			goto tr134
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st25
		}
		goto tr119
	tr134:
//line NONE:1
		te = p + 1

		goto st115
	st115:
		if p++; p == pe {
			goto _test_eof115
		}
	st_case_115:
//line indic_machine.go:3150
		switch data[p] {
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st25
		}
		goto tr119
	st25:
		if p++; p == pe {
			goto _test_eof25
		}
	st_case_25:
		switch data[p] {
		case 7:
			goto tr32
		case 8:
			goto st116
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st24
		}
		goto tr29
	st116:
		if p++; p == pe {
			goto _test_eof116
		}
	st_case_116:
		switch data[p] {
		case 5:
			goto st109
		case 8:
			goto st110
		case 9:
			goto st109
		case 13:
			goto tr32
		case 57:
			goto st110
		}
		goto tr119
	st26:
		if p++; p == pe {
			goto _test_eof26
		}
	st_case_26:
		if data[p] == 13 {
			goto tr32
		}
		goto tr29
	tr34:
//line NONE:1
		te = p + 1

		goto st117
	st117:
		if p++; p == pe {
			goto _test_eof117
		}
	st_case_117:
//line indic_machine.go:3223
		switch data[p] {
		case 3:
			goto tr120
		case 4:
			goto st105
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 16:
			goto tr125
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st27
		}
		goto tr119
	st27:
		if p++; p == pe {
			goto _test_eof27
		}
	st_case_27:
		switch data[p] {
		case 4:
			goto tr30
		case 7:
			goto tr32
		case 8:
			goto st116
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st24
		}
		goto tr29
	tr125:
//line NONE:1
		te = p + 1

		goto st118
	st118:
		if p++; p == pe {
			goto _test_eof118
		}
	st_case_118:
//line indic_machine.go:3277
		switch data[p] {
		case 4:
			goto st119
		case 7:
			goto tr32
		case 8:
			goto st116
		case 9:
			goto st109
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st28
		}
		goto tr119
	st119:
		if p++; p == pe {
			goto _test_eof119
		}
	st_case_119:
		switch data[p] {
		case 5:
			goto tr128
		case 6:
			goto tr138
		case 8:
			goto st108
		case 9:
			goto st109
		case 57:
			goto st108
		}
		goto tr119
	tr128:
//line NONE:1
		te = p + 1

		goto st120
	st120:
		if p++; p == pe {
			goto _test_eof120
		}
	st_case_120:
//line indic_machine.go:3324
		switch data[p] {
		case 8:
			goto st108
		case 9:
			goto st109
		case 57:
			goto st108
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st23
		}
		goto tr119
	tr138:
//line NONE:1
		te = p + 1

		goto st121
	st121:
		if p++; p == pe {
			goto _test_eof121
		}
	st_case_121:
//line indic_machine.go:3347
		switch data[p] {
		case 3:
			goto tr128
		case 8:
			goto st108
		case 9:
			goto st109
		case 57:
			goto st108
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st23
		}
		goto tr119
	st28:
		if p++; p == pe {
			goto _test_eof28
		}
	st_case_28:
		switch data[p] {
		case 4:
			goto tr37
		case 7:
			goto tr32
		case 8:
			goto st116
		case 13:
			goto tr32
		case 57:
			goto st116
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st24
		}
		goto tr29
	tr37:
//line NONE:1
		te = p + 1

		goto st122
	st122:
		if p++; p == pe {
			goto _test_eof122
		}
	st_case_122:
//line indic_machine.go:3393
		switch data[p] {
		case 5:
			goto st23
		case 6:
			goto tr138
		case 8:
			goto st108
		case 9:
			goto st109
		case 57:
			goto st108
		}
		goto tr119
	tr50:
//line NONE:1
		te = p + 1

//line indic_machine.rl:70
		act = 6
		goto st123
	st123:
		if p++; p == pe {
			goto _test_eof123
		}
	st_case_123:
//line indic_machine.go:3419
		switch data[p] {
		case 1:
			goto tr42
		case 2:
			goto tr43
		case 3:
			goto tr44
		case 4:
			goto st81
		case 5:
			goto st15
		case 6:
			goto st20
		case 7:
			goto tr22
		case 8:
			goto tr23
		case 9:
			goto st85
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 15:
			goto tr42
		case 16:
			goto tr52
		case 57:
			goto tr23
		}
		if 10 <= data[p] && data[p] <= 11 {
			goto tr49
		}
		goto tr101
	tr51:
//line NONE:1
		te = p + 1

		goto st124
	st124:
		if p++; p == pe {
			goto _test_eof124
		}
	st_case_124:
//line indic_machine.go:3464
		switch data[p] {
		case 3:
			goto tr57
		case 4:
			goto st125
		case 5:
			goto st6
		case 6:
			goto tr60
		case 7:
			goto tr4
		case 8:
			goto st46
		case 9:
			goto st38
		case 12:
			goto tr8
		case 13:
			goto tr4
		case 16:
			goto tr62
		case 57:
			goto st46
		}
		goto tr56
	st125:
		if p++; p == pe {
			goto _test_eof125
		}
	st_case_125:
		switch data[p] {
		case 1:
			goto tr42
		case 2:
			goto tr43
		case 3:
			goto tr44
		case 4:
			goto st81
		case 5:
			goto tr140
		case 6:
			goto tr141
		case 7:
			goto tr22
		case 8:
			goto st128
		case 9:
			goto st129
		case 11:
			goto tr49
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 15:
			goto tr42
		case 16:
			goto tr52
		case 57:
			goto st128
		}
		goto tr56
	tr140:
//line NONE:1
		te = p + 1

//line indic_machine.rl:65
		act = 1
		goto st126
	st126:
		if p++; p == pe {
			goto _test_eof126
		}
	st_case_126:
//line indic_machine.go:3540
		switch data[p] {
		case 4:
			goto tr20
		case 7:
			goto tr22
		case 8:
			goto st128
		case 9:
			goto st38
		case 12:
			goto tr24
		case 13:
			goto tr22
		case 57:
			goto st128
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st29
		}
		goto tr56
	st29:
		if p++; p == pe {
			goto _test_eof29
		}
	st_case_29:
		switch data[p] {
		case 7:
			goto tr22
		case 8:
			goto st127
		case 13:
			goto tr22
		case 57:
			goto st127
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st17
		}
		goto tr0
	st127:
		if p++; p == pe {
			goto _test_eof127
		}
	st_case_127:
		switch data[p] {
		case 5:
			goto st38
		case 8:
			goto st39
		case 9:
			goto st38
		case 13:
			goto tr22
		case 57:
			goto st39
		}
		goto tr56
	st128:
		if p++; p == pe {
			goto _test_eof128
		}
	st_case_128:
		switch data[p] {
		case 5:
			goto st129
		case 8:
			goto st130
		case 9:
			goto st129
		case 13:
			goto tr22
		case 57:
			goto st130
		}
		goto tr56
	st129:
		if p++; p == pe {
			goto _test_eof129
		}
	st_case_129:
		if data[p] == 9 {
			goto st129
		}
		goto tr56
	st130:
		if p++; p == pe {
			goto _test_eof130
		}
	st_case_130:
		switch data[p] {
		case 5:
			goto st129
		case 9:
			goto st129
		}
		goto tr56
	tr141:
//line NONE:1
		te = p + 1

//line indic_machine.rl:65
		act = 1
		goto st131
	st131:
		if p++; p == pe {
			goto _test_eof131
		}
	st_case_131:
//line indic_machine.go:3649
		switch data[p] {
		case 1:
			goto tr42
		case 3:
			goto tr69
		case 4:
			goto tr20
		case 7:
			goto tr22
		case 8:
			goto st128
		case 9:
			goto st38
		case 13:
			goto tr22
		case 15:
			goto tr42
		case 57:
			goto st128
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st29
		}
		goto tr56
	tr53:
//line NONE:1
		te = p + 1

		goto st132
	st132:
		if p++; p == pe {
			goto _test_eof132
		}
	st_case_132:
//line indic_machine.go:3684
		switch data[p] {
		case 3:
			goto tr147
		case 8:
			goto st134
		case 9:
			goto st135
		case 57:
			goto st134
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st30
		}
		goto tr146
	tr147:
//line NONE:1
		te = p + 1

		goto st133
	st133:
		if p++; p == pe {
			goto _test_eof133
		}
	st_case_133:
//line indic_machine.go:3709
		switch data[p] {
		case 8:
			goto st134
		case 9:
			goto st135
		case 57:
			goto st134
		}
		if 5 <= data[p] && data[p] <= 6 {
			goto st30
		}
		goto tr146
	st30:
		if p++; p == pe {
			goto _test_eof30
		}
	st_case_30:
		switch data[p] {
		case 8:
			goto st134
		case 57:
			goto st134
		}
		goto tr39
	st134:
		if p++; p == pe {
			goto _test_eof134
		}
	st_case_134:
		switch data[p] {
		case 5:
			goto st135
		case 8:
			goto st136
		case 9:
			goto st135
		case 57:
			goto st136
		}
		goto tr146
	st135:
		if p++; p == pe {
			goto _test_eof135
		}
	st_case_135:
		if data[p] == 9 {
			goto st135
		}
		goto tr146
	st136:
		if p++; p == pe {
			goto _test_eof136
		}
	st_case_136:
		switch data[p] {
		case 5:
			goto st135
		case 9:
			goto st135
		}
		goto tr146
	st137:
		if p++; p == pe {
			goto _test_eof137
		}
	st_case_137:
		switch data[p] {
		case 1:
			goto tr42
		case 10:
			goto tr49
		case 15:
			goto tr42
		}
		goto tr118
	st_out:
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
	_test_eof0:
		cs = 0
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
	_test_eof1:
		cs = 1
		goto _test_eof
	_test_eof42:
		cs = 42
		goto _test_eof
	_test_eof2:
		cs = 2
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
	_test_eof3:
		cs = 3
		goto _test_eof
	_test_eof46:
		cs = 46
		goto _test_eof
	_test_eof4:
		cs = 4
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
	_test_eof5:
		cs = 5
		goto _test_eof
	_test_eof50:
		cs = 50
		goto _test_eof
	_test_eof6:
		cs = 6
		goto _test_eof
	_test_eof51:
		cs = 51
		goto _test_eof
	_test_eof52:
		cs = 52
		goto _test_eof
	_test_eof53:
		cs = 53
		goto _test_eof
	_test_eof54:
		cs = 54
		goto _test_eof
	_test_eof55:
		cs = 55
		goto _test_eof
	_test_eof56:
		cs = 56
		goto _test_eof
	_test_eof57:
		cs = 57
		goto _test_eof
	_test_eof58:
		cs = 58
		goto _test_eof
	_test_eof59:
		cs = 59
		goto _test_eof
	_test_eof7:
		cs = 7
		goto _test_eof
	_test_eof60:
		cs = 60
		goto _test_eof
	_test_eof8:
		cs = 8
		goto _test_eof
	_test_eof61:
		cs = 61
		goto _test_eof
	_test_eof62:
		cs = 62
		goto _test_eof
	_test_eof63:
		cs = 63
		goto _test_eof
	_test_eof64:
		cs = 64
		goto _test_eof
	_test_eof65:
		cs = 65
		goto _test_eof
	_test_eof9:
		cs = 9
		goto _test_eof
	_test_eof66:
		cs = 66
		goto _test_eof
	_test_eof67:
		cs = 67
		goto _test_eof
	_test_eof68:
		cs = 68
		goto _test_eof
	_test_eof10:
		cs = 10
		goto _test_eof
	_test_eof69:
		cs = 69
		goto _test_eof
	_test_eof11:
		cs = 11
		goto _test_eof
	_test_eof70:
		cs = 70
		goto _test_eof
	_test_eof71:
		cs = 71
		goto _test_eof
	_test_eof72:
		cs = 72
		goto _test_eof
	_test_eof73:
		cs = 73
		goto _test_eof
	_test_eof12:
		cs = 12
		goto _test_eof
	_test_eof74:
		cs = 74
		goto _test_eof
	_test_eof13:
		cs = 13
		goto _test_eof
	_test_eof75:
		cs = 75
		goto _test_eof
	_test_eof76:
		cs = 76
		goto _test_eof
	_test_eof77:
		cs = 77
		goto _test_eof
	_test_eof14:
		cs = 14
		goto _test_eof
	_test_eof78:
		cs = 78
		goto _test_eof
	_test_eof79:
		cs = 79
		goto _test_eof
	_test_eof80:
		cs = 80
		goto _test_eof
	_test_eof81:
		cs = 81
		goto _test_eof
	_test_eof82:
		cs = 82
		goto _test_eof
	_test_eof15:
		cs = 15
		goto _test_eof
	_test_eof83:
		cs = 83
		goto _test_eof
	_test_eof16:
		cs = 16
		goto _test_eof
	_test_eof84:
		cs = 84
		goto _test_eof
	_test_eof85:
		cs = 85
		goto _test_eof
	_test_eof86:
		cs = 86
		goto _test_eof
	_test_eof87:
		cs = 87
		goto _test_eof
	_test_eof88:
		cs = 88
		goto _test_eof
	_test_eof17:
		cs = 17
		goto _test_eof
	_test_eof89:
		cs = 89
		goto _test_eof
	_test_eof90:
		cs = 90
		goto _test_eof
	_test_eof91:
		cs = 91
		goto _test_eof
	_test_eof18:
		cs = 18
		goto _test_eof
	_test_eof92:
		cs = 92
		goto _test_eof
	_test_eof19:
		cs = 19
		goto _test_eof
	_test_eof93:
		cs = 93
		goto _test_eof
	_test_eof20:
		cs = 20
		goto _test_eof
	_test_eof94:
		cs = 94
		goto _test_eof
	_test_eof95:
		cs = 95
		goto _test_eof
	_test_eof96:
		cs = 96
		goto _test_eof
	_test_eof97:
		cs = 97
		goto _test_eof
	_test_eof21:
		cs = 21
		goto _test_eof
	_test_eof98:
		cs = 98
		goto _test_eof
	_test_eof99:
		cs = 99
		goto _test_eof
	_test_eof100:
		cs = 100
		goto _test_eof
	_test_eof101:
		cs = 101
		goto _test_eof
	_test_eof102:
		cs = 102
		goto _test_eof
	_test_eof103:
		cs = 103
		goto _test_eof
	_test_eof104:
		cs = 104
		goto _test_eof
	_test_eof105:
		cs = 105
		goto _test_eof
	_test_eof106:
		cs = 106
		goto _test_eof
	_test_eof22:
		cs = 22
		goto _test_eof
	_test_eof107:
		cs = 107
		goto _test_eof
	_test_eof23:
		cs = 23
		goto _test_eof
	_test_eof108:
		cs = 108
		goto _test_eof
	_test_eof109:
		cs = 109
		goto _test_eof
	_test_eof110:
		cs = 110
		goto _test_eof
	_test_eof111:
		cs = 111
		goto _test_eof
	_test_eof112:
		cs = 112
		goto _test_eof
	_test_eof24:
		cs = 24
		goto _test_eof
	_test_eof113:
		cs = 113
		goto _test_eof
	_test_eof114:
		cs = 114
		goto _test_eof
	_test_eof115:
		cs = 115
		goto _test_eof
	_test_eof25:
		cs = 25
		goto _test_eof
	_test_eof116:
		cs = 116
		goto _test_eof
	_test_eof26:
		cs = 26
		goto _test_eof
	_test_eof117:
		cs = 117
		goto _test_eof
	_test_eof27:
		cs = 27
		goto _test_eof
	_test_eof118:
		cs = 118
		goto _test_eof
	_test_eof119:
		cs = 119
		goto _test_eof
	_test_eof120:
		cs = 120
		goto _test_eof
	_test_eof121:
		cs = 121
		goto _test_eof
	_test_eof28:
		cs = 28
		goto _test_eof
	_test_eof122:
		cs = 122
		goto _test_eof
	_test_eof123:
		cs = 123
		goto _test_eof
	_test_eof124:
		cs = 124
		goto _test_eof
	_test_eof125:
		cs = 125
		goto _test_eof
	_test_eof126:
		cs = 126
		goto _test_eof
	_test_eof29:
		cs = 29
		goto _test_eof
	_test_eof127:
		cs = 127
		goto _test_eof
	_test_eof128:
		cs = 128
		goto _test_eof
	_test_eof129:
		cs = 129
		goto _test_eof
	_test_eof130:
		cs = 130
		goto _test_eof
	_test_eof131:
		cs = 131
		goto _test_eof
	_test_eof132:
		cs = 132
		goto _test_eof
	_test_eof133:
		cs = 133
		goto _test_eof
	_test_eof30:
		cs = 30
		goto _test_eof
	_test_eof134:
		cs = 134
		goto _test_eof
	_test_eof135:
		cs = 135
		goto _test_eof
	_test_eof136:
		cs = 136
		goto _test_eof
	_test_eof137:
		cs = 137
		goto _test_eof

	_test_eof:
		{
		}
		if p == eof {
			switch cs {
			case 32:
				goto tr56
			case 33:
				goto tr56
			case 34:
				goto tr56
			case 35:
				goto tr56
			case 36:
				goto tr56
			case 0:
				goto tr0
			case 37:
				goto tr56
			case 38:
				goto tr56
			case 39:
				goto tr56
			case 40:
				goto tr56
			case 41:
				goto tr56
			case 1:
				goto tr0
			case 42:
				goto tr56
			case 2:
				goto tr0
			case 43:
				goto tr56
			case 44:
				goto tr56
			case 45:
				goto tr56
			case 3:
				goto tr0
			case 46:
				goto tr56
			case 4:
				goto tr0
			case 47:
				goto tr56
			case 48:
				goto tr56
			case 49:
				goto tr56
			case 5:
				goto tr0
			case 50:
				goto tr56
			case 6:
				goto tr0
			case 51:
				goto tr56
			case 52:
				goto tr56
			case 53:
				goto tr76
			case 54:
				goto tr76
			case 55:
				goto tr76
			case 56:
				goto tr76
			case 57:
				goto tr76
			case 58:
				goto tr76
			case 59:
				goto tr76
			case 7:
				goto tr9
			case 60:
				goto tr76
			case 8:
				goto tr9
			case 61:
				goto tr76
			case 62:
				goto tr76
			case 63:
				goto tr76
			case 64:
				goto tr76
			case 65:
				goto tr76
			case 9:
				goto tr9
			case 66:
				goto tr76
			case 67:
				goto tr76
			case 68:
				goto tr76
			case 10:
				goto tr9
			case 69:
				goto tr76
			case 11:
				goto tr9
			case 70:
				goto tr76
			case 71:
				goto tr76
			case 72:
				goto tr76
			case 73:
				goto tr76
			case 12:
				goto tr9
			case 74:
				goto tr76
			case 13:
				goto tr9
			case 75:
				goto tr76
			case 76:
				goto tr76
			case 77:
				goto tr76
			case 14:
				goto tr9
			case 78:
				goto tr76
			case 79:
				goto tr101
			case 80:
				goto tr101
			case 81:
				goto tr101
			case 82:
				goto tr101
			case 15:
				goto tr19
			case 83:
				goto tr101
			case 16:
				goto tr19
			case 84:
				goto tr101
			case 85:
				goto tr101
			case 86:
				goto tr101
			case 87:
				goto tr101
			case 88:
				goto tr101
			case 17:
				goto tr26
			case 89:
				goto tr101
			case 90:
				goto tr101
			case 91:
				goto tr101
			case 18:
				goto tr19
			case 92:
				goto tr26
			case 19:
				goto tr26
			case 93:
				goto tr101
			case 20:
				goto tr19
			case 94:
				goto tr101
			case 95:
				goto tr101
			case 96:
				goto tr101
			case 97:
				goto tr101
			case 21:
				goto tr19
			case 98:
				goto tr101
			case 99:
				goto tr101
			case 100:
				goto tr118
			case 101:
				goto tr118
			case 102:
				goto tr119
			case 103:
				goto tr119
			case 104:
				goto tr119
			case 105:
				goto tr119
			case 106:
				goto tr119
			case 22:
				goto tr29
			case 107:
				goto tr119
			case 23:
				goto tr29
			case 108:
				goto tr119
			case 109:
				goto tr119
			case 110:
				goto tr119
			case 111:
				goto tr119
			case 112:
				goto tr119
			case 24:
				goto tr29
			case 113:
				goto tr119
			case 114:
				goto tr119
			case 115:
				goto tr119
			case 25:
				goto tr29
			case 116:
				goto tr119
			case 26:
				goto tr29
			case 117:
				goto tr119
			case 27:
				goto tr29
			case 118:
				goto tr119
			case 119:
				goto tr119
			case 120:
				goto tr119
			case 121:
				goto tr119
			case 28:
				goto tr29
			case 122:
				goto tr119
			case 123:
				goto tr101
			case 124:
				goto tr56
			case 125:
				goto tr56
			case 126:
				goto tr56
			case 29:
				goto tr0
			case 127:
				goto tr56
			case 128:
				goto tr56
			case 129:
				goto tr56
			case 130:
				goto tr56
			case 131:
				goto tr56
			case 132:
				goto tr146
			case 133:
				goto tr146
			case 30:
				goto tr39
			case 134:
				goto tr146
			case 135:
				goto tr146
			case 136:
				goto tr146
			case 137:
				goto tr118
			}
		}

	}

//line indic_machine.rl:111

	_ = cs            // Suppress unused variable warning
	_ = foundSyllable // May be unused if no syllables found

	return hasBroken
}
