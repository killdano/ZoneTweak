//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// Performance features of the renderer: hidden-area mask, fixed foveation (NVIDIA variable
// rate shading), reduced-resolution reflection, tiled light lists, depth prepass, render
// scale and frustum culling. Each one falls back to the plain path if the driver or runtime
// doesn't offer what it needs.

const (
	glR8UI               = 0x8232
	glRG32UI             = 0x823C
	glRedInteger         = 0x8D94
	glRGInteger          = 0x8228
	glUnpackAlignment    = 0x0CF5
	glAlways             = 0x0207
	glShadingRateImageNV = 0x9563
	glShadingRateTexelW  = 0x955C
	glShadingRateTexelH  = 0x955D
	glRate1x1NV          = 0x9565
	glRate2x2NV          = 0x9568
	glRate1x2NV          = 0x9566
	glRate4x4NV          = 0x956B
	glUnsignedIntType    = 0x1405
	glUnsignedByteType   = 0x1401
)

var (
	glTexSubImage2DP, glPixelStoreiP, glGetIntegervP, glColorMaskP           uintptr
	glBindShadingRateImageNVP, glShadingRateImagePaletteNVP, glTexStorage2DP uintptr

	renderScale  = 1.0
	foveated     = true
	perfOpt      = true // the optimised path; off draws the way earlier versions did, for comparison
	vrsOK        bool
	vrsTW, vrsTH int32
	wantRebuild  bool
	renderScales = []float64{1.0, 0.9, 0.8, 0.7}
	// foveation: the full-quality circle's radius (degrees from the lens centre) and how much
	// the outside is cut: 0 HIGH (half rate outside), 1 MEDIUM (half, then quarter), 2 LOW
	foveaDeg    = 30.0
	foveaEdge   = 1
	foveaSizes  = []float64{15, 20, 25, 30, 35, 40}
	foveaLabels = []string{"1/2 THEN 1/4", "1/4 THEN 1/16", "1/16"} // shading rate outside the circle
)

func loadPerfGL() {
	core := func(n string) uintptr { return opengl32.NewProc(n).Addr() }
	glTexSubImage2DP = core("glTexSubImage2D")
	glColorMaskP = core("glColorMask")
	glPixelStoreiP = core("glPixelStorei")
	glGetIntegervP = core("glGetIntegerv")
	glBindShadingRateImageNVP = glProc("glBindShadingRateImageNV")
	glShadingRateImagePaletteNVP = glProc("glShadingRateImagePaletteNV")
	glTexStorage2DP = glProc("glTexStorage2D")
	vrsOK = hasGLExt("GL_NV_shading_rate_image") && glBindShadingRateImageNVP != 0 && glShadingRateImagePaletteNVP != 0 && glTexStorage2DP != 0
	if vrsOK {
		var w, h int32
		call(glGetIntegervP, glShadingRateTexelW, uintptr(unsafe.Pointer(&w)))
		call(glGetIntegervP, glShadingRateTexelH, uintptr(unsafe.Pointer(&h)))
		vrsTW, vrsTH = w, h
		if w <= 0 || h <= 0 {
			vrsOK = false
		}
	}
	log.Printf("variable rate shading: %v (texel %dx%d)", vrsOK, vrsTW, vrsTH)
}

func hasGLExt(name string) bool {
	gs := glProc("glGetStringi")
	if gs == 0 {
		return false
	}
	var n int32
	call(glGetIntegervP, 0x821D, uintptr(unsafe.Pointer(&n)))
	for i := int32(0); i < n; i++ {
		p := call(gs, 0x1F03, uintptr(i))
		if p == 0 {
			continue
		}
		if cStr(p) == name {
			return true
		}
	}
	return false
}

func cStr(p uintptr) string {
	base := *(*unsafe.Pointer)(unsafe.Pointer(&p)) // a driver-owned, NUL-terminated string
	var b []byte
	for i := 0; i < 256; i++ {
		c := *(*byte)(unsafe.Add(base, i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

// ---- settings: graphics.txt holds the preset name, then options ----

func loadGfxOpts(dir string) {
	b, err := os.ReadFile(filepath.Join(dir, "spaces", "graphics.txt"))
	if err != nil {
		return
	}
	for _, ln := range strings.Split(string(b), "\n") {
		var f float64
		if n, _ := fmt.Sscanf(strings.TrimSpace(ln), "scale %f", &f); n == 1 && f >= 0.5 && f <= 1 {
			renderScale = f
		}
		if strings.TrimSpace(ln) == "foveated off" {
			foveated = false
		}
		var fd float64
		var fe int
		if n, _ := fmt.Sscanf(strings.TrimSpace(ln), "fovea %f %d", &fd, &fe); n == 2 && fd >= 10 && fd <= 60 && fe >= 0 && fe <= 2 {
			foveaDeg, foveaEdge = fd, fe
		}
		if strings.TrimSpace(ln) == "optimized off" {
			perfOpt = false
		}
	}
}

func saveGfx(dir string) {
	os.MkdirAll(filepath.Join(dir, "spaces"), 0755)
	fv := "on"
	if !foveated {
		fv = "off"
	}
	op := "on"
	if !perfOpt {
		op = "off"
	}
	s := fmt.Sprintf("%s\nscale %.2f\nfoveated %s\noptimized %s\nfovea %.0f %d\n", qualities[qualityIdx].name, renderScale, fv, op, foveaDeg, foveaEdge)
	os.WriteFile(filepath.Join(dir, "spaces", "graphics.txt"), []byte(s), 0644)
}

func scaled(v int32) int32 {
	s := int32(float64(v)*renderScale + 0.5)
	return max(64, s&^1)
}

// ---- per-eye extras ----

type eyeExtras struct {
	refl       rtex
	reflMS     *eyeTarget
	rateTex    uint32
	rateKey    mat4
	rateCfg    [2]float64
	tileHdr    uint32
	tileIdx    uint32
	tiles      lightTiles
	tileIdxRow int32
	hdrW, hdrH int32
}

func (r *renderer) extrasFor(t *eyeTarget) *eyeExtras {
	p := posts[t]
	if p.ex != nil {
		return p.ex
	}
	ex := &eyeExtras{}
	if r.q.mirror && perfOpt {
		// rendered with a little MSAA into its own buffer, resolved into a texture the floor reads
		k := 0.7
		if r.q.name == "MEDIUM" {
			k = 0.5
		}
		rw, rh := int32(float64(t.w)*k), int32(float64(t.h)*k)
		ex.refl = newRT(rw, rh)
		samples := min(int(r.q.samples), 4)
		ms, err := newEyeTargetBest(rw, rh, samples)
		if err != nil {
			log.Printf("reflection target: %v: drawing it in place", err)
			ex.refl.free()
			ex.refl = rtex{}
		} else {
			ex.reflMS = ms
		}
	}
	ex.tileHdr = newTex(1, 1, glRG32UI, glRGInteger, glUnsignedIntType, glNearest)
	ex.tileIdx = newTex(1, 1, glR8UI, glRedInteger, glUnsignedByteType, glNearest)
	p.ex = ex
	return ex
}

func (ex *eyeExtras) free() {
	ex.refl.free()
	if ex.reflMS != nil {
		ex.reflMS.free()
	}
	delOne(glDeleteTexturesP, ex.rateTex)
	delOne(glDeleteTexturesP, ex.tileHdr)
	delOne(glDeleteTexturesP, ex.tileIdx)
}

func freePosts(t *eyeTarget) {
	if p := posts[t]; p != nil {
		p.hdr2.free()
		p.fogT.free()
		for _, b := range p.bloom {
			b.free()
		}
		if p.ex != nil {
			p.ex.free()
		}
		delete(posts, t)
	}
}

// fovea: bind (building if needed) this eye's shading-rate image; false if not in use
func (r *renderer) fovea(t *eyeTarget, ex *eyeExtras, proj mat4) bool {
	if !vrsOK || !foveated || r.q.potato() {
		return false
	}
	cfg := [2]float64{foveaDeg, float64(foveaEdge)}
	if ex.rateTex == 0 || !nearMat(ex.rateKey, proj) || ex.rateCfg != cfg {
		ex.rateCfg = cfg
		delOne(glDeleteTexturesP, ex.rateTex)
		m, nx, ny := rateMap(&proj, int(t.w), int(t.h), int(vrsTW), int(vrsTH), foveaDeg, foveaDeg+12)
		ex.rateTex = genOne(glGenTexturesP)
		call(glBindTextureP, glTexture2D, uintptr(ex.rateTex))
		call(glTexStorage2DP, glTexture2D, 1, glR8UI, uintptr(nx), uintptr(ny))
		call(glPixelStoreiP, glUnpackAlignment, 1)
		call(glTexSubImage2DP, glTexture2D, 0, 0, 0, uintptr(nx), uintptr(ny), glRedInteger, glUnsignedByteType, uintptr(unsafe.Pointer(&m[0])))
		call(glPixelStoreiP, glUnpackAlignment, 4)
		call(glBindTextureP, glTexture2D, 0)
		ex.rateKey = proj
	}
	rates := [3]uint32{glRate1x1NV, glRate2x2NV, glRate4x4NV}
	switch foveaEdge {
	case 0:
		rates = [3]uint32{glRate1x1NV, glRate1x2NV, glRate2x2NV}
	case 2:
		rates = [3]uint32{glRate1x1NV, glRate4x4NV, glRate4x4NV}
	}
	call(glShadingRateImagePaletteNVP, 0, 0, 3, uintptr(unsafe.Pointer(&rates[0])))
	call(glBindShadingRateImageNVP, uintptr(ex.rateTex))
	call(glEnableP, glShadingRateImageNV)
	return true
}

// nearMat: the projection is the same lens setup (ignores tiny per-frame wobble)
func nearMat(a, b mat4) bool {
	for i := range a {
		if d := a[i] - b[i]; d > 0.01 || d < -0.01 {
			return false
		}
	}
	return true
}

func foveaOff() {
	if vrsOK {
		call(glDisableP, glShadingRateImageNV)
	}
}

// uploadTiles: this eye's light lists into its two integer textures; false when tiling
// wouldn't pay off this frame (nearly every light reaches nearly the whole view)
func (ex *eyeExtras) uploadTiles(vp mat4, lights []float32, n int, w, h int32) bool {
	lt := &ex.tiles
	if !lt.build(&vp, lights, n, int(w), int(h)) {
		return false
	}
	call(glPixelStoreiP, glUnpackAlignment, 1)
	rows := int32(len(lt.idx) / tileIdxW)
	call(glBindTextureP, glTexture2D, uintptr(ex.tileHdr))
	if ex.hdrW == int32(lt.tx) && ex.hdrH == int32(lt.ty) {
		call(glTexSubImage2DP, glTexture2D, 0, 0, 0, uintptr(lt.tx), uintptr(lt.ty), glRGInteger, glUnsignedIntType, uintptr(unsafe.Pointer(&lt.hdr[0])))
	} else {
		call(glTexImage2DP, glTexture2D, 0, glRG32UI, uintptr(lt.tx), uintptr(lt.ty), 0, glRGInteger, glUnsignedIntType, uintptr(unsafe.Pointer(&lt.hdr[0])))
		ex.hdrW, ex.hdrH = int32(lt.tx), int32(lt.ty)
	}
	call(glBindTextureP, glTexture2D, uintptr(ex.tileIdx))
	if rows <= ex.tileIdxRow {
		call(glTexSubImage2DP, glTexture2D, 0, 0, 0, tileIdxW, uintptr(rows), glRedInteger, glUnsignedByteType, uintptr(unsafe.Pointer(&lt.idx[0])))
	} else {
		// grown: allocate with headroom so it rarely has to grow again
		ex.tileIdxRow = rows + 8
		call(glTexImage2DP, glTexture2D, 0, glR8UI, tileIdxW, uintptr(ex.tileIdxRow), 0, glRedInteger, glUnsignedByteType, 0)
		call(glTexSubImage2DP, glTexture2D, 0, 0, 0, tileIdxW, uintptr(rows), glRedInteger, glUnsignedByteType, uintptr(unsafe.Pointer(&lt.idx[0])))
	}
	call(glBindTextureP, glTexture2D, 0)
	call(glPixelStoreiP, glUnpackAlignment, 4)
	return true
}

func (r *renderer) bindTiles(p *prog, ex *eyeExtras, on bool) {
	bindTex(5, ex.tileHdr)
	bindTex(6, ex.tileIdx)
	p.i1("u_tileHdr", 5)
	p.i1("u_tileIdx", 6)
	p.f1("u_tilePx", tilePx)
	v := 0
	if on {
		v = 1
	}
	p.i1("u_tiled", v)
}

// ---- hidden-area mask ----

func (r *renderer) drawMask(eye int, proj mat4) {
	if eye < 0 || eye > 1 || len(xs.mask[eye]) == 0 {
		return
	}
	if r.maskVer[eye] != xs.maskVer {
		delOne(glDeleteBuffersP, r.maskVBO[eye])
		r.maskVBO[eye], r.maskVer[eye] = 0, xs.maskVer
	}
	if r.maskVBO[eye] == 0 {
		call(glGenBuffersP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
		r.maskVBO[eye] = gIDs[0]
		v := xs.mask[eye]
		call(glBindBufferP, glArrayBuffer, uintptr(r.maskVBO[eye]))
		call(glBufferDataP, glArrayBuffer, uintptr(len(v)*4), uintptr(unsafe.Pointer(&v[0])), glStaticDraw)
	}
	call(glUseProgramP, uintptr(r.mask.id))
	r.mask.m4("u_proj", proj)
	call(glBindBufferP, glArrayBuffer, uintptr(r.maskVBO[eye]))
	for i := uintptr(1); i < 5; i++ {
		call(glDisableVertexAttribArrayP, i)
	}
	call(glEnableVertexAttribArrayP, 0)
	call(glVertexAttribPointerP, 0, 3, glFloatType, 0, 12, 0)
	call(glDrawArraysP, glTriangles, 0, uintptr(len(xs.mask[eye])/3))
}

// ---- culled drawing of the static room and resting generators ----

var runBuf [][2]int

// slideOn: the wall-slide mapping applies to the room's static geometry only, never to the
// generators, robots or anything else that moves on its own
func slideOn(p *prog, on bool) {
	if slideActive() {
		v := 0
		if on {
			v = 1
		}
		p.i1("u_slideOn", v)
	}
}

func (r *renderer) drawStatic(p *prog, vp *mat4) {
	g, ok := gpu[&bStatic]
	if !ok || g.n == 0 {
		return
	}
	slideOn(p, true)
	defer slideOn(p, false)
	call(glBindBufferP, glArrayBuffer, uintptr(g.vbo))
	bindAttribs(false)
	if vp == nil || len(r.staticChunks) == 0 || slideActive() {
		call(glDrawArraysP, glTriangles, 0, uintptr(g.n))
		return
	}
	runBuf = visibleRuns(vp, r.staticChunks, runBuf)
	for _, rn := range runBuf {
		call(glDrawArraysP, glTriangles, uintptr(rn[0]), uintptr(rn[1]))
	}
}

func drawGenPartsIn(vp *mat4) {
	for _, p := range genParts {
		g := genVBOs[p.key]
		if g == nil || g.n == 0 {
			continue
		}
		if vp != nil && !boxVisible(vp, g.box) {
			continue
		}
		call(glBindBufferP, glArrayBuffer, uintptr(g.vbo))
		bindAttribs(false)
		call(glDrawArraysP, glTriangles, 0, uintptr(g.n))
	}
}

// setSlide: the wall-slide mapping for the room's vertices (off unless a wall is moving)
func setSlide(p *prog) {
	if !slideActive() {
		p.i1("u_slideOn", 0)
		return
	}
	g := envG
	p.i1("u_slideOn", 0) // switched on only around the room's own geometry (slideOn)
	p.f3("u_sO", v3(g.tx, 0, g.tz))
	p.f3("u_sX", v3(g.a, 0, g.c))
	p.f3("u_sZ", v3(g.b, 0, g.d))
	o, n := slide.old, slide.new
	p.f4v("u_sOld", []float32{float32(o[0]), float32(o[1]), float32(o[2]), float32(o[3])})
	p.f4v("u_sNew", []float32{float32(n[0]), float32(n[1]), float32(n[2]), float32(n[3])})
	p.f1("u_sK", float32(slideK()))
}
