# HarfBuzz Shape Tests for textshape

This directory contains the HarfBuzz shape tests ported to test textshape's shaping functionality.

## Source

Tests and fonts are copied from HarfBuzz's test suite:
- Source: `harfbuzz/test/shape/data/in-house/`
- 84 test files with 5,131 individual test cases
- 203 test fonts

## Running Tests

```bash
# Run all HarfBuzz tests
go test ./harfbuzz-tests/...

# Run with verbose output
go test -v ./harfbuzz-tests/...

# Run a specific test file
HB_TEST_FILE=simple.tests go test -v ./harfbuzz-tests/... -run TestSingleFile
```

## Current Status

As of initial integration:

| Metric | Count | Percentage |
|--------|-------|------------|
| Total Tests | 5,131 | 100% |
| Passed | 152 | 3.0% |
| Failed | 4,978 | 97.0% |
| Skipped | 1 | 0.0% |

### Test Files Status

**Fully Passing:**
- `simple.tests` - Basic Latin shaping (1/2, fallback shaper skipped)
- `color-fonts.tests` - Color font handling
- `language-tags.tests` - Language tag processing
- `rand.tests` - Randomization features
- `rotation.tests` - Glyph rotation

**Partially Passing (>50%):**
- `variations-rvrn.tests` - 49/100
- `per-script-kern-fallback.tests` - 10/12
- `tibetan-vowels.tests` - 10/11

**Known Limitations:**

1. **Complex Script Shapers Not Implemented:**
   - Arabic (joining, normalization)
   - Indic scripts (syllable processing, reordering)
   - Thai/Khmer/Myanmar (syllable state machines)
   - Hebrew diacritics

2. **Missing Features:**
   - AAT (Apple Advanced Typography) tables: MORX, TRAK
   - Fallback shaper
   - Vertical text layout
   - Some variation features (RVRN)

3. **Partial Support:**
   - Unicode normalization for Arabic
   - Default ignorables handling
   - Cluster level handling

## Test Format

Tests use HarfBuzz's semicolon-separated format:
```
font_path;options;unicode_input;expected_output
```

### Options
- `--shaper=ot` - Use OpenType shaper
- `--direction=l|r|t|b` - Text direction
- `--variations=axis=value` - Variable font settings
- `--features=feat1,feat2` - OpenType features
- `--no-positions` - Skip position checking
- `--no-clusters` - Skip cluster checking
- `--cluster-level=0|1|2|3` - Cluster handling mode

### Output Format
```
[GlyphName=cluster+advance|GlyphName=cluster@xoff,yoff+advance|...]
```

## Implementation Roadmap

To achieve full HarfBuzz compatibility:

1. **Arabic Shaper** - Joining logic, normalization
2. **Indic Shaper** - Syllable recognition, reordering
3. **USE (Universal Shaping Engine)** - General complex script support
4. **Unicode Bidi** - Bidirectional text handling
5. **AAT Support** - Apple font features

## License

Test files and fonts are from HarfBuzz (MIT license).
