// Copyright 2014 Hajime Hoshi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opengl

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hajimehoshi/ebiten/internal/graphics"
)

type shaderID int

const (
	shaderVertexModelview shaderID = iota
	shaderFragmentColorMatrix
)

// glslReservedKeywords is a set of reserved keywords that cannot be used as an indentifier on some environments.
// See https://www.khronos.org/registry/OpenGL/specs/gl/GLSLangSpec.4.60.pdf.
var glslReservedKeywords = map[string]struct{}{
	"common": {}, "partition": {}, "active": {},
	"asm":   {},
	"class": {}, "union": {}, "enum": {}, "typedef": {}, "template": {}, "this": {},
	"resource": {},
	"goto":     {},
	"inline":   {}, "noinline": {}, "public": {}, "static": {}, "extern": {}, "external": {}, "interface": {},
	"long": {}, "short": {}, "half": {}, "fixed": {}, "unsigned": {}, "superp": {},
	"input": {}, "output": {},
	"hvec2": {}, "hvec3": {}, "hvec4": {}, "fvec2": {}, "fvec3": {}, "fvec4": {},
	"filter": {},
	"sizeof": {}, "cast": {},
	"namespace": {}, "using": {},
	"sampler3DRect": {},
}

var glslIdentifier = regexp.MustCompile(`[_a-zA-Z][_a-zA-Z0-9]*`)

func checkGLSL(src string) {
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "//") {
			l = l[:strings.Index(l, "//")]
		}
		for _, token := range glslIdentifier.FindAllString(l, -1) {
			if _, ok := glslReservedKeywords[token]; ok {
				panic(fmt.Sprintf("opengl: %q is a reserved keyword", token))
			}
		}
	}
}

func shaderStr(id shaderID) string {
	src := ""
	switch id {
	case shaderVertexModelview:
		src = shaderStrVertex
	case shaderFragmentColorMatrix:
		replaces := map[string]string{
			"{{.FilterNearest}}":      fmt.Sprintf("%d", graphics.FilterNearest),
			"{{.FilterLinear}}":       fmt.Sprintf("%d", graphics.FilterLinear),
			"{{.FilterScreen}}":       fmt.Sprintf("%d", graphics.FilterScreen),
			"{{.AddressClampToZero}}": fmt.Sprintf("%d", graphics.AddressClampToZero),
			"{{.AddressRepeat}}":      fmt.Sprintf("%d", graphics.AddressRepeat),
		}
		src = shaderStrFragment
		for k, v := range replaces {
			src = strings.Replace(src, k, v, -1)
		}
	default:
		panic("not reached")
	}

	checkGLSL(src)
	return src
}

const (
	shaderStrVertex = `
uniform vec2 viewport_size;
attribute vec2 vertex;
attribute vec2 tex;
attribute vec4 tex_region;
attribute vec4 color_scale;
varying vec2 varying_tex;
varying vec4 varying_tex_region;
varying vec4 varying_color_scale;

void main(void) {
  varying_tex = tex;
  varying_tex_region = tex_region;
  varying_color_scale = color_scale;

  mat4 projection_matrix = mat4(
    vec4(2.0 / viewport_size.x, 0, 0, 0),
    vec4(0, 2.0 / viewport_size.y, 0, 0),
    vec4(0, 0, 1, 0),
    vec4(-1, -1, 0, 1)
  );
  gl_Position = projection_matrix * vec4(vertex, 0, 1);
}
`
	shaderStrFragment = `
#if defined(GL_ES)
precision mediump float;
#else
#define lowp
#define mediump
#define highp
#endif

#define FILTER_NEAREST ({{.FilterNearest}})
#define FILTER_LINEAR ({{.FilterLinear}})
#define FILTER_SCREEN ({{.FilterScreen}})
#define ADDRESS_CLAMP_TO_ZERO ({{.AddressClampToZero}})
#define ADDRESS_REPEAT ({{.AddressRepeat}})

uniform sampler2D texture;
uniform mat4 color_matrix_body;
uniform vec4 color_matrix_translation;

uniform int filter_type;
uniform highp vec2 source_size;
uniform int address;

#if defined(FILTER_SCREEN)
uniform highp float scale;
#endif

varying highp vec2 varying_tex;
varying highp vec4 varying_tex_region;
varying highp vec4 varying_color_scale;

// adjustTexel adjusts the two texels and returns the adjusted second texel.
// When p1 - p0 is exactly equal to the texel size, jaggy can happen on macOS (#669).
// In order to avoid this jaggy, subtract a little bit from the second texel.
highp vec2 adjustTexel(highp vec2 p0, highp vec2 p1) {
  highp vec2 texel_size = 1.0 / source_size;
  if (fract((p1.x-p0.x)*source_size.x) == 0.0) {
    p1.x -= texel_size.x / 512.0;
  }
  if (fract((p1.y-p0.y)*source_size.y) == 0.0) {
    p1.y -= texel_size.y / 512.0;
  }
  return p1;
}

highp float floorMod(highp float x, highp float y) {
  if (x < 0.0) {
    return y - (-x - y * floor(-x/y));
  }
  return x - y * floor(x/y);
}

highp vec2 adjustTexelByAddress(highp vec2 p, highp vec4 tex_region, int address) {
  if (address == ADDRESS_CLAMP_TO_ZERO) {
    return p;
  }
  if (address == ADDRESS_REPEAT) {
    highp vec2 o = vec2(tex_region[0], tex_region[1]);
    highp vec2 size = vec2(tex_region[2] - tex_region[0], tex_region[3] - tex_region[1]);
    return vec2(floorMod((p.x - o.x), size.x) + o.x, floorMod((p.y - o.y), size.y) + o.y);
  }
  // Not reached.
  return vec2(0.0);
}

void main(void) {
  highp vec2 pos = varying_tex;
  highp vec2 texel_size = 1.0 / source_size;

  vec4 color;

  if (filter_type == FILTER_NEAREST) {
    pos = adjustTexelByAddress(pos, varying_tex_region, address);
    color = texture2D(texture, pos);
    if (pos.x < varying_tex_region[0] ||
      pos.y < varying_tex_region[1] ||
      (varying_tex_region[2] - texel_size.x / 512.0) <= pos.x ||
      (varying_tex_region[3] - texel_size.y / 512.0) <= pos.y) {
      color = vec4(0, 0, 0, 0);
    }
  } else if (filter_type == FILTER_LINEAR) {
    highp vec2 p0 = pos - texel_size / 2.0;
    highp vec2 p1 = pos + texel_size / 2.0;

    p1 = adjustTexel(p0, p1);
    p0 = adjustTexelByAddress(p0, varying_tex_region, address);
    p1 = adjustTexelByAddress(p1, varying_tex_region, address);

    vec4 c0 = texture2D(texture, p0);
    vec4 c1 = texture2D(texture, vec2(p1.x, p0.y));
    vec4 c2 = texture2D(texture, vec2(p0.x, p1.y));
    vec4 c3 = texture2D(texture, p1);
    if (p0.x < varying_tex_region[0]) {
      c0 = vec4(0, 0, 0, 0);
      c2 = vec4(0, 0, 0, 0);
    }
    if (p0.y < varying_tex_region[1]) {
      c0 = vec4(0, 0, 0, 0);
      c1 = vec4(0, 0, 0, 0);
    }
    if ((varying_tex_region[2] - texel_size.x / 512.0) <= p1.x) {
      c1 = vec4(0, 0, 0, 0);
      c3 = vec4(0, 0, 0, 0);
    }
    if ((varying_tex_region[3] - texel_size.y / 512.0) <= p1.y) {
      c2 = vec4(0, 0, 0, 0);
      c3 = vec4(0, 0, 0, 0);
    }

    vec2 rate = fract(p0 * source_size);
    color = mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y);
  } else if (filter_type == FILTER_SCREEN) {
    highp vec2 p0 = pos - texel_size / 2.0 / scale;
    highp vec2 p1 = pos + texel_size / 2.0 / scale;

    p1 = adjustTexel(p0, p1);

    vec4 c0 = texture2D(texture, p0);
    vec4 c1 = texture2D(texture, vec2(p1.x, p0.y));
    vec4 c2 = texture2D(texture, vec2(p0.x, p1.y));
    vec4 c3 = texture2D(texture, p1);
    // Texels must be in the source rect, so it is not necessary to check that like linear filter.

    vec2 rateCenter = vec2(1.0, 1.0) - texel_size / 2.0 / scale;
    vec2 rate = clamp(((fract(p0 * source_size) - rateCenter) * scale) + rateCenter, 0.0, 1.0);
    color = mix(mix(c0, c1, rate.x), mix(c2, c3, rate.x), rate.y);
  } else {
    // Not reached.
    discard;
  }

  // Un-premultiply alpha
  if (0.0 < color.a) {
    color.rgb /= color.a;
  }
  // Apply the color matrix or scale.
  color = (color_matrix_body * color) + color_matrix_translation;
  color *= varying_color_scale;
  color = clamp(color, 0.0, 1.0);
  // Premultiply alpha
  color.rgb *= color.a;

  gl_FragColor = color;
}
`
)
