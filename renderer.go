//go:build windows

package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

//go:embed shaders/*
var shaderFS embed.FS

const (
	glArrayBuffer        = 0x8892
	glStreamDraw         = 0x88E0
	glStaticDraw         = 0x88E4
	glVertexShader       = 0x8B31
	glFragmentShader     = 0x8B30
	glCompileStatus      = 0x8B81
	glLinkStatus         = 0x8B82
	glRGBA16F            = 0x881A
	glHalfFloat          = 0x140B
	glDepthComponent     = 0x1902
	glTexture0           = 0x84C0
	glTexCompareMode     = 0x884C
	glCompareRefToTex    = 0x884E
	glTexCompareFunc     = 0x884D
	glLequal             = 0x0203
	glNearest            = 0x2600
	glOne                = 1
	glLess               = 0x0201
	glDepthFuncLEqual    = 0x0203
	glUnsignedInt        = 0x1405
	glLineStrip          = 0x0003
	glFloatType          = 0x1406
	glClampToBorder      = 0x812D
	glTexBorderColor     = 0x1004
	glFramebufferBinding = 0x8CA6
)

var (
	glCreateShaderP, glShaderSourceP, glCompileShaderP, glGetShaderivP, glGetShaderInfoLogP  uintptr
	glCreateProgramP, glAttachShaderP, glLinkProgramP, glGetProgramivP, glGetProgramInfoLogP uintptr
	glUseProgramP, glGetUniformLocationP, glUniform1iP, glUniform1fvP, glUniform3fvP         uintptr
	glUniform4fvP, glUniformMatrix4fvP, glGenBuffersP, glBindBufferP, glBufferDataP          uintptr
	glVertexAttribPointerP, glEnableVertexAttribArrayP, glDisableVertexAttribArrayP          uintptr
	glGenVertexArraysP, glBindVertexArrayP, glActiveTextureP, glDrawBuffersP, glDepthFuncP   uintptr
	glUniform2fvP, glTexParameterfvP                                                         uintptr
)

type prog struct {
	id   uint32
	locs map[string]int32
}

func (p *prog) loc(name string) int32 {
	if l, ok := p.locs[name]; ok {
		return l
	}
	b := append([]byte(name), 0)
	l := int32(call(glGetUniformLocationP, uintptr(p.id), uintptr(unsafe.Pointer(&b[0]))))
	p.locs[name] = l
	return l
}

var (
	uf  = new([256]float32)
	uf4 = new([maxLights * 12]float32)
)

func (p *prog) i1(name string, v int) {
	if l := p.loc(name); l >= 0 {
		call(glUniform1iP, uintptr(l), uintptr(v))
	}
}
func (p *prog) f1(name string, v float32) {
	if l := p.loc(name); l >= 0 {
		uf[0] = v
		call(glUniform1fvP, uintptr(l), 1, uintptr(unsafe.Pointer(&uf[0])))
	}
}
func (p *prog) f2(name string, x, y float32) {
	if l := p.loc(name); l >= 0 {
		uf[0], uf[1] = x, y
		call(glUniform2fvP, uintptr(l), 1, uintptr(unsafe.Pointer(&uf[0])))
	}
}
func (p *prog) f3(name string, v vec3) {
	if l := p.loc(name); l >= 0 {
		uf[0], uf[1], uf[2] = float32(v.x), float32(v.y), float32(v.z)
		call(glUniform3fvP, uintptr(l), 1, uintptr(unsafe.Pointer(&uf[0])))
	}
}
func (p *prog) f4v(name string, v []float32) {
	if l := p.loc(name); l >= 0 && len(v) >= 4 {
		copy(uf4[:], v)
		call(glUniform4fvP, uintptr(l), uintptr(len(v)/4), uintptr(unsafe.Pointer(&uf4[0])))
	}
}
func (p *prog) m4(name string, m mat4) {
	if l := p.loc(name); l >= 0 {
		copy(uf[:16], m[:])
		call(glUniformMatrix4fvP, uintptr(l), 1, 0, uintptr(unsafe.Pointer(&uf[0])))
	}
}
func (p *prog) m4v(name string, ms []mat4) {
	if l := p.loc(name); l >= 0 && len(ms) > 0 {
		for i, m := range ms {
			copy(uf[i*16:], m[:])
		}
		call(glUniformMatrix4fvP, uintptr(l), uintptr(len(ms)), 0, uintptr(unsafe.Pointer(&uf[0])))
	}
}

func shaderSrc(name string, common bool) string {
	b, err := shaderFS.ReadFile("shaders/" + name)
	if err != nil {
		panic(err)
	}
	s := "#version 330 core\n"
	if common {
		c, _ := shaderFS.ReadFile("shaders/common.glsl")
		s += string(c) + "\n"
	}
	return s + string(b)
}

func compileShader(kind uintptr, src string) (uint32, error) {
	sh := uint32(call(glCreateShaderP, kind))
	cs := append([]byte(src), 0)
	ptr := uintptr(unsafe.Pointer(&cs[0]))
	pp := &ptr
	call(glShaderSourceP, uintptr(sh), 1, uintptr(unsafe.Pointer(pp)), 0)
	runtime.KeepAlive(cs)
	call(glCompileShaderP, uintptr(sh))
	var ok int32
	call(glGetShaderivP, uintptr(sh), glCompileStatus, uintptr(unsafe.Pointer(&ok)))
	if ok == 0 {
		buf := make([]byte, 8192)
		var n int32
		call(glGetShaderInfoLogP, uintptr(sh), 8192, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
		return 0, fmt.Errorf("%s", strings.TrimSpace(string(buf[:n])))
	}
	return sh, nil
}

func makeProg(vs, fs string, common bool) (*prog, error) {
	v, err := compileShader(glVertexShader, shaderSrc(vs, false))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", vs, err)
	}
	f, err := compileShader(glFragmentShader, shaderSrc(fs, common))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", fs, err)
	}
	id := uint32(call(glCreateProgramP))
	call(glAttachShaderP, uintptr(id), uintptr(v))
	call(glAttachShaderP, uintptr(id), uintptr(f))
	call(glLinkProgramP, uintptr(id))
	var ok int32
	call(glGetProgramivP, uintptr(id), glLinkStatus, uintptr(unsafe.Pointer(&ok)))
	if ok == 0 {
		buf := make([]byte, 8192)
		var n int32
		call(glGetProgramInfoLogP, uintptr(id), 8192, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
		return nil, fmt.Errorf("link %s+%s: %s", vs, fs, string(buf[:n]))
	}
	return &prog{id: id, locs: map[string]int32{}}, nil
}

type rtex struct {
	fbo, tex uint32
	w, h     int32
}

type renderer struct {
	lit, floor, glow, sprite, smoke, flame, shadow, fog, fogcomp, bdown, bup, final *prog
	mask, depth                                                                     *prog
	maskVBO                                                                         [2]uint32
	maskVer                                                                         [2]int
	staticChunks                                                                    []chunk
	vao                                                                             uint32
	vbos                                                                            map[any]uint32
	shadowFBO, shadowTex                                                            [2]uint32
	shadowCacheFBO, shadowCacheTex                                                  [2]uint32
	shadowCacheKey                                                                  [2]shadowKey
	shadowRes                                                                       int32
	hdr2, fogT                                                                      rtex
	bloom                                                                           []rtex
	frame                                                                           float32
	q                                                                               quality
	staticVer                                                                       int
	uploadedStatic                                                                  int
	uploadedGen                                                                     int
}

type quality struct {
	name                 string
	fogDiv               int32
	fogSteps, fogLights  int
	samples, shadow      int32
	bloomLevels          int
	fogDensity, bloomAmt float32
	exposure             float32
	maxLights            int
	mirror               bool
}

var qualities = []quality{
	{"ULTRA", 4, 32, 16, 8, 4096, 7, 0.035, 0.07, 1.35, 48, true},
	{"HIGH", 5, 28, 14, 4, 2048, 6, 0.035, 0.07, 1.35, 40, true},
	{"MEDIUM", 6, 18, 8, 2, 1024, 5, 0.033, 0.06, 1.35, 28, true},
	{"LOW", 8, 10, 5, 2, 1024, 4, 0.03, 0.05, 1.35, 14, false},
	{"POTATO", 12, 5, 2, 0, 256, 3, 0.022, 0.04, 1.35, 6, false},
}

func (q quality) potato() bool { return q.name == "POTATO" }

func (q quality) idx() int {
	for i, x := range qualities {
		if x.name == q.name {
			return i
		}
	}
	return 0
}

var (
	R           *renderer
	wantQuality = -1
	qualityIdx  int
	appDir      string
)

func loadGL33() error {
	ps := map[string]*uintptr{
		"glCreateShader": &glCreateShaderP, "glShaderSource": &glShaderSourceP, "glCompileShader": &glCompileShaderP,
		"glGetShaderiv": &glGetShaderivP, "glGetShaderInfoLog": &glGetShaderInfoLogP, "glCreateProgram": &glCreateProgramP,
		"glAttachShader": &glAttachShaderP, "glLinkProgram": &glLinkProgramP, "glGetProgramiv": &glGetProgramivP,
		"glGetProgramInfoLog": &glGetProgramInfoLogP, "glUseProgram": &glUseProgramP, "glGetUniformLocation": &glGetUniformLocationP,
		"glUniform1i": &glUniform1iP, "glUniform1fv": &glUniform1fvP, "glUniform2fv": &glUniform2fvP, "glUniform3fv": &glUniform3fvP,
		"glUniform4fv": &glUniform4fvP, "glUniformMatrix4fv": &glUniformMatrix4fvP, "glGenBuffers": &glGenBuffersP, "glDeleteBuffers": &glDeleteBuffersP,
		"glBindBuffer": &glBindBufferP, "glBufferData": &glBufferDataP, "glVertexAttribPointer": &glVertexAttribPointerP,
		"glEnableVertexAttribArray": &glEnableVertexAttribArrayP, "glDisableVertexAttribArray": &glDisableVertexAttribArrayP,
		"glGenVertexArrays": &glGenVertexArraysP, "glBindVertexArray": &glBindVertexArrayP, "glActiveTexture": &glActiveTextureP,
		"glDrawBuffers": &glDrawBuffersP,
	}
	for n, p := range ps {
		*p = glProc(n)
		if *p == 0 {
			return fmt.Errorf("OpenGL 3.3 function %s missing; update your GPU driver", n)
		}
	}
	glDepthFuncP = opengl32.NewProc("glDepthFunc").Addr()
	glTexParameterfvP = opengl32.NewProc("glTexParameterfv").Addr()
	return nil
}

func newRenderer(qi int) (*renderer, error) {
	if err := loadGL33(); err != nil {
		return nil, err
	}
	r := &renderer{vbos: map[any]uint32{}, q: qualities[qi]}
	potatoMode = r.q.potato()
	var err error
	mk := func(dst **prog, vs, fs string, common bool) {
		if err != nil {
			return
		}
		*dst, err = makeProg(vs, fs, common)
	}
	mk(&r.lit, "lit.vs", "lit.fs", true)
	mk(&r.floor, "lit.vs", "floor.fs", true)
	mk(&r.glow, "glow.vs", "glow.fs", false)
	mk(&r.sprite, "sprite.vs", "sprite.fs", false)
	mk(&r.smoke, "smoke.vs", "smoke.fs", true)
	mk(&r.flame, "smoke.vs", "flame.fs", true)
	mk(&r.shadow, "shadow.vs", "shadow.fs", false)
	mk(&r.fog, "post.vs", "fog.fs", true)
	mk(&r.fogcomp, "post.vs", "fogcomp.fs", false)
	mk(&r.bdown, "post.vs", "bloomdown.fs", false)
	mk(&r.bup, "post.vs", "bloomup.fs", false)
	mk(&r.final, "post.vs", "final.fs", false)
	mk(&r.mask, "mask.vs", "mask.fs", false)
	mk(&r.depth, "lit.vs", "depth.fs", false)
	if err != nil {
		return nil, err
	}
	loadPerfGL()
	gt.init()
	// fixed texture units for every sampler the shared shader code declares, so no two
	// samplers of different kinds ever default onto the same unit
	for _, p := range []*prog{r.lit, r.floor, r.smoke, r.flame, r.fog} {
		call(glUseProgramP, uintptr(p.id))
		p.i1("u_shadow0", 1)
		p.i1("u_shadow1", 2)
		p.i1("u_tileHdr", 5)
		p.i1("u_tileIdx", 6)
		p.i1("u_refl", 7)
		p.i1("u_tiled", 0)
	}
	call(glUseProgramP, 0)
	call(glGenVertexArraysP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
	r.vao = gIDs[0]
	call(glBindVertexArrayP, uintptr(r.vao))
	r.makeShadowMaps()
	log.Printf("renderer: %s quality, %d MSAA, shadow %d, fog 1/%d x %d steps", r.q.name, r.q.samples, r.q.shadow, r.q.fogDiv, r.q.fogSteps)
	return r, nil
}

func newTex(w, h int32, internal, format, typ uintptr, filter uintptr) uint32 {
	call(glGenTexturesP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
	t := gIDs[0]
	call(glBindTextureP, glTexture2D, uintptr(t))
	call(glTexParameteriP, glTexture2D, glTexMinFilter, filter)
	call(glTexParameteriP, glTexture2D, glTexMagFilter, filter)
	call(glTexParameteriP, glTexture2D, glTexMaxLevel, 0)
	call(glTexParameteriP, glTexture2D, glTexWrapS, glClampToEdge)
	call(glTexParameteriP, glTexture2D, glTexWrapT, glClampToEdge)
	call(glTexImage2DP, glTexture2D, 0, internal, uintptr(w), uintptr(h), 0, format, typ, 0)
	call(glBindTextureP, glTexture2D, 0)
	return t
}

func newRT(w, h int32) rtex {
	t := rtex{w: w, h: h}
	t.tex = newTex(w, h, glRGBA16F, glRGBA, glHalfFloat, glLinear)
	t.fbo = genOne(glGenFramebuffersP)
	call(glBindFramebufferP, glFramebuffer, uintptr(t.fbo))
	call(glFramebufferTexture2DP, glFramebuffer, glColorAttachment0, glTexture2D, uintptr(t.tex), 0)
	call(glBindFramebufferP, glFramebuffer, 0)
	return t
}

// setQuality switches every setting at once: shadow maps and post buffers are rebuilt here,
// the eye targets (MSAA) by the caller
func (r *renderer) setQuality(qi int) {
	r.q = qualities[qi]
	potatoMode = r.q.potato()
	for i := 0; i < 2; i++ {
		delOne(glDeleteTexturesP, r.shadowTex[i])
		if glDeleteFramebuffersP == 0 {
			glDeleteFramebuffersP, glDeleteRenderbuffersP = glProc("glDeleteFramebuffers"), glProc("glDeleteRenderbuffers")
		}
		delOne(glDeleteFramebuffersP, r.shadowFBO[i])
		delOne(glDeleteTexturesP, r.shadowCacheTex[i])
		delOne(glDeleteFramebuffersP, r.shadowCacheFBO[i])
		r.shadowCacheKey[i] = shadowKey{}
	}
	r.makeShadowMaps()
	for t := range posts {
		freePosts(t)
	}
	log.Printf("renderer: switched to %s", r.q.name)
}

func (r *renderer) makeShadowMaps() {
	r.shadowRes = r.q.shadow
	for i := 0; i < 2; i++ {
		t := newTex(r.shadowRes, r.shadowRes, glDepthComponent24, glDepthComponent, glUnsignedInt, glLinear)
		call(glBindTextureP, glTexture2D, uintptr(t))
		call(glTexParameteriP, glTexture2D, glTexCompareMode, glCompareRefToTex)
		call(glTexParameteriP, glTexture2D, glTexCompareFunc, glLequal)
		call(glBindTextureP, glTexture2D, 0)
		r.shadowTex[i] = t
		f := genOne(glGenFramebuffersP)
		call(glBindFramebufferP, glFramebuffer, uintptr(f))
		call(glFramebufferTexture2DP, glFramebuffer, glDepthAttachment, glTexture2D, uintptr(t), 0)
		call(opengl32.NewProc("glDrawBuffer").Addr(), 0)
		call(opengl32.NewProc("glReadBuffer").Addr(), 0)
		if s := uint32(call(glCheckFramebufferStatusP, glFramebuffer)); s != glFramebufferOK {
			log.Printf("shadow map %d incomplete (0x%x)", i, s)
		}
		r.shadowFBO[i] = f
		// the static part of each shadow map (room + resting generators), rendered only when
		// that geometry or the lamp changes and copied in before the moving things each frame
		ct := newTex(r.shadowRes, r.shadowRes, glDepthComponent24, glDepthComponent, glUnsignedInt, glNearest)
		cf := genOne(glGenFramebuffersP)
		call(glBindFramebufferP, glFramebuffer, uintptr(cf))
		call(glFramebufferTexture2DP, glFramebuffer, glDepthAttachment, glTexture2D, uintptr(ct), 0)
		call(opengl32.NewProc("glDrawBuffer").Addr(), 0)
		call(opengl32.NewProc("glReadBuffer").Addr(), 0)
		r.shadowCacheTex[i], r.shadowCacheFBO[i] = ct, cf
		r.shadowCacheKey[i] = shadowKey{}
	}
	call(glBindFramebufferP, glFramebuffer, 0)
}

type shadowKey struct {
	static, gen int
	vp          mat4
	ok          bool
}

type postSet struct {
	hdr2, fogT rtex
	bloom      []rtex
	ex         *eyeExtras
}

var posts = map[*eyeTarget]*postSet{}

func (r *renderer) ensureTargets(t *eyeTarget) {
	if p := posts[t]; p != nil {
		r.hdr2, r.fogT, r.bloom = p.hdr2, p.fogT, p.bloom
		return
	}
	p := &postSet{hdr2: newRT(t.w, t.h), fogT: newRT(t.w/r.q.fogDiv, t.h/r.q.fogDiv)}
	bw, bh := t.w/2, t.h/2
	for i := 0; i < r.q.bloomLevels && bw > 8 && bh > 8; i++ {
		p.bloom = append(p.bloom, newRT(bw, bh))
		bw, bh = bw/2, bh/2
	}
	posts[t] = p
	r.hdr2, r.fogT, r.bloom = p.hdr2, p.fogT, p.bloom
}

func (r *renderer) upload(key any, v []float32) (uint32, int) {
	id, ok := r.vbos[key]
	if !ok {
		call(glGenBuffersP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
		id = gIDs[0]
		r.vbos[key] = id
	}
	call(glBindBufferP, glArrayBuffer, uintptr(id))
	if len(v) > 0 {
		call(glBufferDataP, glArrayBuffer, uintptr(len(v)*4), uintptr(unsafe.Pointer(&v[0])), glStreamDraw)
	}
	return id, len(v)
}

type gpuBatch struct {
	vbo uint32
	n   int
	tex bool
}

var gpu = map[any]gpuBatch{}

func (r *renderer) uploadAll() {
	for _, b := range []*batch{&bEnv, &bEnvL, &bFloor, &bFloorL, &bGlowTri, &bGlowLine, &bGlowWide, &bLines, &bTrans, &bOpaque} {
		id, n := r.upload(b, b.v)
		gpu[b] = gpuBatch{id, n / vstride, false}
	}
	if r.uploadedStatic != r.staticVer || gpu[&bStatic].vbo == 0 {
		// the room is sorted into 2 m columns so each eye only draws the ones in view
		r.staticChunks = chunkify(bStatic.v, 2)
		for _, b := range []*batch{&bStatic, &bStaticL, &bFloorS, &bFloorQ} {
			id, n := r.upload(b, b.v)
			gpu[b] = gpuBatch{id, n / vstride, false}
		}
		r.uploadedStatic = r.staticVer
	}
	// static generator pieces: one buffer each, uploaded once, freed when unused for a while
	genUploadN++
	for _, p := range genParts {
		g := genVBOs[p.key]
		if g == nil {
			call(glGenBuffersP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
			g = &genVBO{vbo: gIDs[0], n: len(p.v) / vstride, box: boxOf(p.v)}
			call(glBindBufferP, glArrayBuffer, uintptr(g.vbo))
			call(glBufferDataP, glArrayBuffer, uintptr(len(p.v)*4), uintptr(unsafe.Pointer(&p.v[0])), glStaticDraw)
			genVBOs[p.key] = g
		}
		g.used = genUploadN
	}
	if genUploadN%30 == 0 && glDeleteBuffersP != 0 {
		for k, g := range genVBOs {
			if genUploadN-g.used > 30 {
				gIDs[0] = g.vbo
				call(glDeleteBuffersP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
				delete(genVBOs, k)
			}
		}
	}
	for _, b := range []*tbatch{&bSoft, &bShadow, &bSmoke, &bFire} {
		id, n := r.upload(b, b.v)
		gpu[b] = gpuBatch{id, n / 9, true}
	}
}

func bindAttribs(tex bool) {
	if tex {
		call(glEnableVertexAttribArrayP, 0)
		call(glEnableVertexAttribArrayP, 1)
		call(glEnableVertexAttribArrayP, 2)
		call(glDisableVertexAttribArrayP, 3)
		call(glDisableVertexAttribArrayP, 4)
		call(glVertexAttribPointerP, 0, 3, glFloatType, 0, 36, 0)
		call(glVertexAttribPointerP, 1, 2, glFloatType, 0, 36, 12)
		call(glVertexAttribPointerP, 2, 4, glFloatType, 0, 36, 20)
		return
	}
	for i := uintptr(0); i < 5; i++ {
		call(glEnableVertexAttribArrayP, i)
	}
	call(glVertexAttribPointerP, 0, 3, glFloatType, 0, vstride*4, 0)
	call(glVertexAttribPointerP, 1, 3, glFloatType, 0, vstride*4, 12)
	call(glVertexAttribPointerP, 2, 4, glFloatType, 0, vstride*4, 24)
	call(glVertexAttribPointerP, 3, 4, glFloatType, 0, vstride*4, 40)
	call(glVertexAttribPointerP, 4, 3, glFloatType, 0, vstride*4, 56)
}

type genVBO struct {
	vbo  uint32
	n    int
	used int
	box  aabb
}

var (
	genVBOs          = map[any]*genVBO{}
	genUploadN       int
	glDeleteBuffersP uintptr
)

func drawB(key any, mode uintptr) {
	g, ok := gpu[key]
	if !ok || g.n == 0 {
		return
	}
	call(glBindBufferP, glArrayBuffer, uintptr(g.vbo))
	bindAttribs(g.tex)
	call(glDrawArraysP, mode, 0, uintptr(g.n))
}

func (r *renderer) setScene(p *prog, cam vec3, lights []float32, nl int) {
	p.f1("u_time", float32(fxTime()))
	cheap := float32(0)
	if r.q.potato() {
		cheap = 1
	}
	p.f1("u_cheap", cheap)
	p.f3("u_cam", cam)
	p.f4v("u_L", lights)
	p.i1("u_nL", nl)
	ns := len(spots)
	if ns > 2 {
		ns = 2
	}
	p.i1("u_nS", ns)
	var sp, sd, sc []float32
	var sm []mat4
	for i := 0; i < 2; i++ {
		s := spotLight{}
		if i < ns {
			s = spots[i]
		}
		sp = append(sp, float32(s.pos.x), float32(s.pos.y), float32(s.pos.z), float32(s.cosOut))
		sd = append(sd, float32(s.dir.x), float32(s.dir.y), float32(s.dir.z), float32(s.cosIn))
		sc = append(sc, float32(s.col[0]), float32(s.col[1]), float32(s.col[2]), float32(s.rng))
		sm = append(sm, s.vp)
	}
	p.f4v("u_Spos", sp)
	p.f4v("u_Sdir", sd)
	p.f4v("u_Scol", sc)
	p.m4v("u_SM", sm)
	p.f1("u_shadowTexel", 1/float32(r.shadowRes))
	call(glActiveTextureP, glTexture0+1)
	call(glBindTextureP, glTexture2D, uintptr(r.shadowTex[0]))
	call(glActiveTextureP, glTexture0+2)
	call(glBindTextureP, glTexture2D, uintptr(r.shadowTex[1]))
	call(glActiveTextureP, glTexture0)
	p.i1("u_shadow0", 1)
	p.i1("u_shadow1", 2)
}

var lightBuf []float32

func (r *renderer) beginFrame(cam vec3) int {
	r.frame++
	gt.frame()
	r.uploadAll()
	var n int
	lightBuf, n = packLights(cam, lightBuf)
	if n > r.q.maxLights {
		n = r.q.maxLights
	}
	call(glBindVertexArrayP, uintptr(r.vao))
	call(glEnableP, glDepthTest)
	call(glDisableP, glBlend)
	call(glDepthMaskP, 1)
	for i, s := range spots {
		if i >= 2 {
			break
		}
		call(glBindFramebufferP, glFramebuffer, uintptr(r.shadowFBO[i]))
		call(glViewportP, 0, 0, uintptr(r.shadowRes), uintptr(r.shadowRes))
		call(glClearP, glDepthBufferBit)
		if r.q.potato() {
			continue // cleared shadow map = everything lit, no shadow pass at all
		}
		call(glUseProgramP, uintptr(r.shadow.id))
		r.shadow.m4("u_vp", s.vp)
		key := shadowKey{static: r.staticVer, gen: genPartsVer, vp: s.vp, ok: true}
		if r.shadowCacheKey[i] != key {
			r.shadowCacheKey[i] = key
			call(glBindFramebufferP, glFramebuffer, uintptr(r.shadowCacheFBO[i]))
			call(glClearP, glDepthBufferBit)
			r.drawStatic(nil)
			drawGenPartsIn(nil)
		}
		call(glBindFramebufferP, glReadFramebuffer, uintptr(r.shadowCacheFBO[i]))
		call(glBindFramebufferP, glDrawFramebuffer, uintptr(r.shadowFBO[i]))
		sr := uintptr(r.shadowRes)
		call(glBlitFramebufferP, 0, 0, sr, sr, 0, 0, sr, sr, glDepthBufferBit, glNearest)
		call(glBindFramebufferP, glFramebuffer, uintptr(r.shadowFBO[i]))
		drawB(&bEnv, glTriangles)
	}
	gt.mark("SHADOW")
	return n
}

func (r *renderer) post(p *prog) { call(glUseProgramP, uintptr(p.id)) }

func fullscreen() {
	for i := uintptr(0); i < 5; i++ {
		call(glDisableVertexAttribArrayP, i)
	}
	call(glDrawArraysP, glTriangles, 0, 3)
}

func bindTex(unit int, tex uint32) {
	call(glActiveTextureP, glTexture0+uintptr(unit))
	call(glBindTextureP, glTexture2D, uintptr(tex))
	call(glActiveTextureP, glTexture0)
}

func (r *renderer) drawGlowPass(vp mat4, wide bool) {
	call(glEnableP, glBlend)
	call(glBlendFuncP, glOne, glOne)
	call(glDepthMaskP, 0)
	call(glUseProgramP, uintptr(r.glow.id))
	r.glow.m4("u_vp", vp)
	r.glow.f1("u_gain", 3.0)
	drawB(&bGlowTri, glTriangles)
	call(glLineWidthP, f32(1.5))
	drawB(&bGlowLine, glLines)
	if wide {
		call(glLineWidthP, f32(5))
		drawB(&bGlowWide, glLines)
	}
	call(glUseProgramP, uintptr(r.sprite.id))
	r.sprite.m4("u_vp", vp)
	r.sprite.f1("u_gain", 3.5)
	r.sprite.f1("u_dark", 0)
	drawB(&bSoft, glTriangles)
	call(glDepthMaskP, 1)
	call(glBlendFuncP, glSrcAlpha, glOneMinusSrcAlpha)
}

// drawMirror: the room flipped under the deck, cheap shading, its glow added on top
func (r *renderer) drawMirror(ex *eyeExtras, cam vec3, nl int, vpm mat4, cull *mat4) {
	call(glUseProgramP, uintptr(r.lit.id))
	r.setScene(r.lit, cam, lightBuf, nl)
	r.bindTiles(r.lit, ex, false)
	r.lit.f3("u_cam", vec3{cam.x, -cam.y, cam.z})
	r.lit.f1("u_emisGain", 4)
	r.lit.f1("u_alphaMode", 0)
	r.lit.m4("u_vp", vpm)
	r.lit.f1("u_cheap", 1)
	if nl > 12 {
		r.lit.i1("u_nL", 12)
	}
	r.drawStatic(cull)
	drawGenPartsIn(cull)
	drawB(&bEnv, glTriangles)
	r.drawGlowPass(vpm, false)
}

func (r *renderer) renderEye(eye int, t *eyeTarget, proj mat4, view hmdMatrix34, cam vec3, nl int, floorY float32, dst uint32, dw, dh int32) {
	for i := 0; i < 3; i++ {
		view[i][3] += view[i][1] * floorY
	}
	mirror := view
	for i := 0; i < 3; i++ {
		mirror[i][1] = -mirror[i][1]
	}
	vp := mul4(proj, mat4From34(view))
	vpm := mul4(proj, mat4From34(mirror))
	r.ensureTargets(t)
	ex := r.extrasFor(t)
	cheap := float32(0)
	if r.q.potato() {
		cheap = 1
	}
	tiled := perfOpt && !r.q.potato() && ex.uploadTiles(vp, lightBuf, nl, t.w, t.h)
	cv, cvm := &vp, &vpm // culling matrices (nil draws everything)
	if !perfOpt {
		cv, cvm = nil, nil
	}

	// the reflection: the mirrored room at reduced resolution, plain materials and the nearest
	// lights (it's only ever seen dimmed through the wet deck)
	inPlace := r.q.mirror && (ex.refl.fbo == 0 || !perfOpt)
	if r.q.mirror && !inPlace {
		m := ex.reflMS
		call(glBindFramebufferP, glFramebuffer, uintptr(m.msFBO))
		call(glViewportP, 0, 0, uintptr(m.w), uintptr(m.h))
		call(glClearColorP, 0, 0, 0, 0)
		call(glClearP, glColorBufferBit|glDepthBufferBit)
		call(glEnableP, glDepthTest)
		call(glEnableP, glMultisample)
		call(glDisableP, glBlend)
		call(glDepthFuncP, glLess)
		r.drawMask(eye, proj)
		r.drawMirror(ex, cam, nl, vpm, cvm)
		call(glDisableP, glBlend)
		call(glBindFramebufferP, glReadFramebuffer, uintptr(m.msFBO))
		call(glBindFramebufferP, glDrawFramebuffer, uintptr(ex.refl.fbo))
		call(glBlitFramebufferP, 0, 0, uintptr(m.w), uintptr(m.h), 0, 0, uintptr(m.w), uintptr(m.h), glColorBufferBit, glNearest)
		gt.mark("REFLECT")
	}

	call(glBindFramebufferP, glFramebuffer, uintptr(t.msFBO))
	call(glViewportP, 0, 0, uintptr(t.w), uintptr(t.h))
	call(glClearColorP, 0, 0, 0, 0)
	call(glClearP, glColorBufferBit|glDepthBufferBit)
	call(glEnableP, glDepthTest)
	call(glEnableP, glMultisample)
	call(glDisableP, glBlend)
	call(glDepthFuncP, glLess)
	if perfOpt {
		r.drawMask(eye, proj)
	}
	if inPlace {
		r.drawMirror(ex, cam, nl, vpm, cvm)
	}
	if r.frame < 4 {
		if e := uint32(call(opengl32.NewProc("glGetError").Addr())); e != 0 {
			log.Printf("GL error before frame %v: 0x%x", r.frame, e)
		}
	}

	// depth prepass: the room, generators and everything opaque that moves, depth only, so the
	// expensive shading below runs once per visible pixel
	call(glDisableP, glBlend)
	if perfOpt {
		call(glColorMaskP, 0, 0, 0, 0)
		call(glUseProgramP, uintptr(r.depth.id))
		r.depth.m4("u_vp", vp)
		r.drawStatic(cv)
		drawGenPartsIn(cv)
		drawB(&bEnv, glTriangles)
		call(glColorMaskP, 1, 1, 1, 1)
		gt.mark("PREPASS")
	}
	if perfOpt {
		call(glDepthFuncP, glDepthFuncLEqual)
	}
	fov := perfOpt && r.fovea(t, ex, proj)

	call(glEnableP, glBlend)
	call(glBlendFuncP, glSrcAlpha, glOneMinusSrcAlpha)
	call(glUseProgramP, uintptr(r.floor.id))
	r.setScene(r.floor, cam, lightBuf, nl)
	r.bindTiles(r.floor, ex, tiled)
	r.floor.m4("u_vp", vp)
	if r.q.mirror {
		r.floor.f1("u_reflScale", 1)
	} else {
		r.floor.f1("u_reflScale", 0.3)
	}
	refl := float32(0)
	if r.q.mirror && !inPlace {
		refl = 1
		bindTex(7, ex.refl.tex)
		r.floor.i1("u_refl", 7)
	}
	r.floor.f1("u_reflTex", refl)
	r.floor.f2("u_screen", float32(t.w), float32(t.h))
	r.floor.f3("u_fo", floorFrame.o)
	r.floor.f3("u_fx", floorFrame.x)
	r.floor.f3("u_fz", floorFrame.z)
	drawB(&bFloorQ, glTriangles)

	call(glUseProgramP, uintptr(r.lit.id))
	r.setScene(r.lit, cam, lightBuf, nl)
	r.bindTiles(r.lit, ex, tiled)
	r.lit.m4("u_vp", vp)
	r.lit.f1("u_alphaMode", 1)
	r.lit.f1("u_emisGain", 4)
	call(glDepthMaskP, 0)
	drawB(&bFloorS, glTriangles)
	drawB(&bFloor, glTriangles)
	call(glUseProgramP, uintptr(r.sprite.id))
	r.sprite.m4("u_vp", vp)
	r.sprite.f1("u_dark", 1)
	drawB(&bShadow, glTriangles)

	// opaque: already in the depth buffer, so no depth writes
	call(glDisableP, glBlend)
	call(glUseProgramP, uintptr(r.lit.id))
	r.lit.f3("u_cam", cam)
	r.lit.f1("u_cheap", cheap)
	r.lit.f1("u_alphaMode", 0)
	if !perfOpt {
		call(glDepthMaskP, 1) // no prepass: these lay down their own depth
	}
	r.drawStatic(cv)
	drawGenPartsIn(cv)
	drawB(&bEnv, glTriangles)
	call(glDepthMaskP, 1)
	r.lit.f1("u_emisGain", 1.3)
	drawB(&bOpaque, glTriangles)
	call(glLineWidthP, f32(1))
	drawB(&bEnvL, glLines)
	drawB(&bStaticL, glLines)
	call(glEnableP, glBlend)
	call(glBlendFuncP, glSrcAlpha, glOneMinusSrcAlpha)
	r.lit.f1("u_alphaMode", 1)
	call(glLineWidthP, f32(2))
	drawB(&bLines, glLines)
	drawB(&bFloorL, glLines)
	call(glDepthMaskP, 0)
	call(glUseProgramP, uintptr(r.smoke.id))
	r.setScene(r.smoke, cam, lightBuf, nl)
	r.smoke.m4("u_vp", vp)
	r.smoke.f1("u_gain", 1.0)
	call(glBlendFuncP, glOne, glOneMinusSrcAlpha)
	drawB(&bSmoke, glTriangles)
	if len(bFire.v) > 0 {
		call(glUseProgramP, uintptr(r.flame.id))
		r.flame.f1("u_time", float32(fxTime()))
		r.flame.m4("u_vp", vp)
		call(glBlendFuncP, glOne, glOne)
		drawB(&bFire, glTriangles)
	}
	call(glBlendFuncP, glSrcAlpha, glOneMinusSrcAlpha)
	call(glDepthMaskP, 1)
	call(glUseProgramP, uintptr(r.lit.id))
	r.lit.f1("u_emisGain", 1.0)
	drawB(&bTrans, glTriangles)
	r.lit.f1("u_emisGain", 4)
	r.drawGlowPass(vp, true)
	call(glDisableP, glBlend)
	if fov {
		foveaOff()
	}
	call(glDepthFuncP, glLess)
	gt.mark("SCENE")

	// resolve colour + depth
	call(glBindFramebufferP, glReadFramebuffer, uintptr(t.msFBO))
	call(glBindFramebufferP, glDrawFramebuffer, uintptr(t.resolveFBO))
	call(glBlitFramebufferP, 0, 0, uintptr(t.w), uintptr(t.h), 0, 0, uintptr(t.w), uintptr(t.h), glColorBufferBit, glNearest)
	call(glBindFramebufferP, glReadFramebuffer, uintptr(t.msFBO))
	call(glBindFramebufferP, glDrawFramebuffer, uintptr(t.depthFBO))
	call(glBlitFramebufferP, 0, 0, uintptr(t.w), uintptr(t.h), 0, 0, uintptr(t.w), uintptr(t.h), glDepthBufferBit, glNearest)
	call(glDisableP, glDepthTest)
	call(glDisableP, glMultisample)

	// volumetric fog
	call(glBindFramebufferP, glFramebuffer, uintptr(r.fogT.fbo))
	call(glViewportP, 0, 0, uintptr(r.fogT.w), uintptr(r.fogT.h))
	r.post(r.fog)
	r.setScene(r.fog, cam, lightBuf, nl)
	bindTex(0, t.depthTex)
	r.fog.i1("u_depth", 0)
	r.fog.m4("u_ivp", inv4(vp))
	r.fog.f1("u_density", r.q.fogDensity)
	r.fog.i1("u_steps", r.q.fogSteps)
	r.fog.i1("u_fogLights", r.q.fogLights)
	r.fog.f1("u_maxDist", 12)
	r.fog.f1("u_frame", r.frame)
	fa, fb, nf := packFog()
	r.fog.f4v("u_FV", fa)
	r.fog.f4v("u_FVp", fb)
	r.fog.i1("u_nFV", nf)
	fullscreen()
	gt.mark("FOG")

	call(glBindFramebufferP, glFramebuffer, uintptr(r.hdr2.fbo))
	call(glViewportP, 0, 0, uintptr(t.w), uintptr(t.h))
	r.post(r.fogcomp)
	bindTex(0, t.tex)
	bindTex(3, r.fogT.tex)
	r.fogcomp.i1("u_scene", 0)
	r.fogcomp.i1("u_fog", 3)
	fullscreen()

	// bloom chain
	src := r.hdr2.tex
	r.post(r.bdown)
	for i, b := range r.bloom {
		call(glBindFramebufferP, glFramebuffer, uintptr(b.fbo))
		call(glViewportP, 0, 0, uintptr(b.w), uintptr(b.h))
		bindTex(0, src)
		r.bdown.i1("u_src", 0)
		first := 0
		if i == 0 {
			first = 1
		}
		r.bdown.i1("u_first", first)
		fullscreen()
		src = b.tex
	}
	r.post(r.bup)
	call(glEnableP, glBlend)
	call(glBlendFuncP, glOne, glOne)
	for i := len(r.bloom) - 1; i > 0; i-- {
		d := r.bloom[i-1]
		call(glBindFramebufferP, glFramebuffer, uintptr(d.fbo))
		call(glViewportP, 0, 0, uintptr(d.w), uintptr(d.h))
		bindTex(0, r.bloom[i].tex)
		r.bup.i1("u_src", 0)
		r.bup.f1("u_radius", 1)
		fullscreen()
	}
	call(glDisableP, glBlend)

	// tone map straight into the headset image at its full size (this is also where a reduced
	// render scale gets scaled back up)
	call(glBindFramebufferP, glFramebuffer, uintptr(dst))
	call(glViewportP, 0, 0, uintptr(dw), uintptr(dh))
	r.post(r.final)
	bindTex(0, r.hdr2.tex)
	r.final.i1("u_scene", 0)
	if len(r.bloom) > 0 {
		bindTex(4, r.bloom[0].tex)
	}
	r.final.i1("u_bloom", 4)
	r.final.f1("u_bloomAmt", r.q.bloomAmt)
	r.final.f1("u_exposure", r.q.exposure)
	r.final.f1("u_frame", r.frame)
	fullscreen()
	gt.mark("POST")
	call(glUseProgramP, 0)
	call(glEnableP, glDepthTest)
	if r.frame < 4 {
		if e := uint32(call(opengl32.NewProc("glGetError").Addr())); e != 0 {
			log.Printf("GL error after frame %v: 0x%x", r.frame, e)
		}
	}
}

func loadQuality(dir string) int {
	loadGfxOpts(dir)
	b, err := os.ReadFile(filepath.Join(dir, "spaces", "graphics.txt"))
	if err != nil {
		return 0
	}
	first := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	for i, q := range qualities {
		if first == q.name {
			return i
		}
	}
	return 0
}

func saveQuality(dir string, i int) {
	qualityIdx = i
	saveGfx(dir)
}
