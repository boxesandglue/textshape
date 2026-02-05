# textshape

A pure Go text shaping engine and font subsetter. The shaping logic is a port of [HarfBuzz](https://harfbuzz.github.io/).

> **Status:** Beta. The library is actively used in [boxes and glue](https://github.com/boxesandglue/boxesandglue) for PDF typesetting. The API may still change.

## Features

- **OpenType shaping**: Full GSUB and GPOS support (all lookup types)
- **Complex scripts**: Arabic, Indic (Devanagari, Bengali, Gujarati, ...), Khmer, Myanmar, Hebrew, Thai, Hangul, USE
- **Variable fonts**: fvar, gvar, HVAR, avar
- **Vertical text**: vmtx, VORG, vertical origins
- **Synthetic bold/slant**: HarfBuzz-compatible API
- **Font subsetting**: Reduce fonts to needed glyphs, with variable font instancing
- **CFF support**: CFF/CFF2 shaping and subsetting with subroutine optimization
- **Kern fallback**: Legacy kern table when no GPOS kerning

## Installation

```bash
go get github.com/boxesandglue/textshape
```

## Example

```go
package main

import (
    "fmt"
    "os"

    "github.com/boxesandglue/textshape/ot"
)

func main() {
    data, _ := os.ReadFile("MyFont.ttf")
    font, _ := ot.ParseFont(data, 0)
    shaper, _ := ot.NewShaper(font)

    buf := ot.NewBuffer()
    buf.AddString("office")
    buf.GuessSegmentProperties()
    shaper.Shape(buf, nil)

    for i, info := range buf.Info {
        fmt.Printf("glyph=%d advance=%d\n", info.GlyphID, buf.Pos[i].XAdvance)
    }
}
```

## Documentation

Full documentation with examples is available at: https://boxesandglue.dev/textshape/

- [Getting Started](https://boxesandglue.dev/textshape/gettingstarted/) — Font loading, buffer, shaping, results
- [Buffer](https://boxesandglue.dev/textshape/buffer/) — Direction, script, language, flags
- [Features](https://boxesandglue.dev/textshape/features/) — OpenType feature control
- [Variable Fonts](https://boxesandglue.dev/textshape/variablefonts/) — Axis variations and instancing
- [Synthetic Bold/Slant](https://boxesandglue.dev/textshape/syntheticboldslant/) — Faux bold and italic
- [Font Subsetting](https://boxesandglue.dev/textshape/subsetting/) — Reduce fonts for embedding
- [API Reference](https://boxesandglue.dev/textshape/reference/) — Complete type and method reference

## Hacking

### Running tests

```bash
# Unit tests
go test ./ot/...
go test ./subset/...

# HarfBuzz compatibility test suite (84 test files)
go test ./harfbuzz-tests/...

# Verbose output showing per-file results
go test -v ./harfbuzz-tests/...

# Run a single test file
HB_TEST_FILE="arabic-fallback-shaping.tests" go test -v -run TestSingleFile ./harfbuzz-tests/
```

### HarfBuzz compatibility

The shaping engine aims for **identical output** to HarfBuzz. The test suite in `harfbuzz-tests/` runs the same test cases as HarfBuzz's own test suite (`hb-shape` format). Each `.tests` file in `harfbuzz-tests/tests/` contains lines like:

```
../fonts/Roboto-Regular.ttf;;U+006F,U+0066,U+0066,U+0069,U+0063,U+0065;[o=0+1171|ffi=1+1551|c=4+1013|e=5+1107]
```

Tests are automatically skipped when the required font is not available or when the font file SHA1 hash does not match (font version pinning). This means the test suite runs on any machine — tests requiring unavailable fonts are skipped rather than failing.

### Project structure

```
ot/               Main shaping package (Shaper, Buffer, Font, Face, ...)
subset/           Font subsetting and variable font instancing
harfbuzz-tests/   HarfBuzz compatibility test suite
  tests/          .tests files (HarfBuzz hb-shape format)
  fonts/          Test fonts (some via symlink)
```

### Key design decisions

- **No cgo**: Pure Go, no HarfBuzz C dependency
- **Font units only**: The shaper works in font units (no scaling). Callers scale by `fontSize / upem`.
- **Shaper reuse**: A `Shaper` is created once per font and reused across shaping calls. Settings like synthetic bold, variations, and default features persist between calls.
- **int16 positions**: Glyph positions use `int16` (matching the OpenType spec's typical value range), not `int32` or `float`.

## License

MIT License — see [LICENSE](LICENSE)

## Acknowledgments

- [HarfBuzz](https://harfbuzz.github.io/) — The reference text shaping engine
- [textlayout](https://github.com/benoitkugler/textlayout) — Earlier Go HarfBuzz port that inspired this implementation
