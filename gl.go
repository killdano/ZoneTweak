//go:build windows

package main

import (
	"fmt"
	"log"
	"unsafe"
)

const (
	glColorBufferBit   = 0x4000
	glDepthBufferBit   = 0x0100
	glDepthTest        = 0x0B71
	glBlend            = 0x0BE2
	glSrcAlpha         = 0x0302
	glOneMinusSrcAlpha = 0x0303
	glLines            = 0x0001
	glTriangles        = 0x0004
	glFloat            = 0x1406
	glVertexArray      = 0x8074
	glColorArray       = 0x8076
	glProjection       = 0x1701
	glModelview        = 0x1700
	glTexture2D        = 0x0DE1
	glRGBA8            = 0x8058
	glRGBA             = 0x1908
	glUnsignedByte     = 0x1401
	glTexMinFilter     = 0x2801
	glTexMagFilter     = 0x2800
	glTexMaxLevel      = 0x813D
	glLinear           = 0x2601
	glFramebuffer      = 0x8D40
	glReadFramebuffer  = 0x8CA8
	glDrawFramebuffer  = 0x8CA9
	glRenderbuffer     = 0x8D41
	glColorAttachment0 = 0x8CE0
	glDepthAttachment  = 0x8D00
	glDepthComponent24 = 0x81A6
	glFramebufferOK    = 0x8CD5
	glMultisample      = 0x809D
	glLineSmooth       = 0x0B20
	glTexCoordArray    = 0x8078
	glFog              = 0x0B60
	glFogMode          = 0x0B65
	glExp2             = 0x0801
	glFogDensity       = 0x0B62
	glFogColor         = 0x0B66
	glTexWrapS         = 0x2802
	glTexWrapT         = 0x2803
	glClampToEdge      = 0x812F
)

var (
	glClearColorP, glClearP, glViewportP, glEnableP, glDisableP, glBlendFuncP     uintptr
	glMatrixModeP, glLoadMatrixfP, glEnableClientStateP, glDisableClientStateP    uintptr
	glVertexPointerP, glColorPointerP, glDrawArraysP, glLineWidthP, glDepthMaskP  uintptr
	glGenTexturesP, glBindTextureP, glTexImage2DP, glTexParameteriP, glFlushP     uintptr
	glGenFramebuffersP, glBindFramebufferP, glFramebufferTexture2DP               uintptr
	glGenRenderbuffersP, glBindRenderbufferP, glRenderbufferStorageMultisampleP   uintptr
	glRenderbufferStorageP, glFramebufferRenderbufferP, glCheckFramebufferStatusP uintptr
	glBlitFramebufferP, wglSwapIntervalEXTP                                       uintptr
	glTexCoordPointerP, glFogiP, glFogfP, glFogfvP                                uintptr
	glowTex                                                                       uint32
	fogCol                                                                        = new([4]float32)
	gIDs                                                                          = new([8]uint32)
	gMat                                                                          = new([16]float32)
)

func loadGL() error {
	core := func(n string) uintptr { return opengl32.NewProc(n).Addr() }
	glClearColorP = core("glClearColor")
	glClearP = core("glClear")
	glViewportP = core("glViewport")
	glEnableP = core("glEnable")
	glDisableP = core("glDisable")
	glBlendFuncP = core("glBlendFunc")
	glMatrixModeP = core("glMatrixMode")
	glLoadMatrixfP = core("glLoadMatrixf")
	glEnableClientStateP = core("glEnableClientState")
	glDisableClientStateP = core("glDisableClientState")
	glVertexPointerP = core("glVertexPointer")
	glColorPointerP = core("glColorPointer")
	glDrawArraysP = core("glDrawArrays")
	glLineWidthP = core("glLineWidth")
	glDepthMaskP = core("glDepthMask")
	glGenTexturesP = core("glGenTextures")
	glBindTextureP = core("glBindTexture")
	glTexImage2DP = core("glTexImage2D")
	glTexParameteriP = core("glTexParameteri")
	glFlushP = core("glFlush")
	glTexCoordPointerP = core("glTexCoordPointer")
	glFogiP = core("glFogi")
	glFogfP = core("glFogf")
	glFogfvP = core("glFogfv")
	ext := map[string]*uintptr{
		"glGenFramebuffers": &glGenFramebuffersP, "glBindFramebuffer": &glBindFramebufferP,
		"glFramebufferTexture2D": &glFramebufferTexture2DP, "glGenRenderbuffers": &glGenRenderbuffersP,
		"glBindRenderbuffer": &glBindRenderbufferP, "glRenderbufferStorageMultisample": &glRenderbufferStorageMultisampleP,
		"glRenderbufferStorage": &glRenderbufferStorageP, "glFramebufferRenderbuffer": &glFramebufferRenderbufferP,
		"glCheckFramebufferStatus": &glCheckFramebufferStatusP, "glBlitFramebuffer": &glBlitFramebufferP,
	}
	for n, p := range ext {
		*p = glProc(n)
		if *p == 0 {
			*p = glProc(n + "EXT")
		}
		if *p == 0 && n != "glRenderbufferStorageMultisample" {
			return fmt.Errorf("OpenGL function %s is missing; update your GPU driver", n)
		}
	}
	wglSwapIntervalEXTP = glProc("wglSwapIntervalEXT")
	if wglSwapIntervalEXTP != 0 {
		call(wglSwapIntervalEXTP, 0)
	}
	return nil
}

type eyeTarget struct {
	msFBO, resolveFBO, tex uint32
	depthFBO, depthTex     uint32
	colorRB, depthRB       uint32
	w, h                   int32
}

var (
	glDeleteFramebuffersP  uintptr
	glDeleteRenderbuffersP uintptr
	glDeleteTexturesP      = opengl32.NewProc("glDeleteTextures").Addr()
)

func delOne(p uintptr, id uint32) {
	if p != 0 && id != 0 {
		gIDs[0] = id
		call(p, 1, uintptr(unsafe.Pointer(&gIDs[0])))
	}
}

func (e *eyeTarget) free() {
	if glDeleteFramebuffersP == 0 {
		glDeleteFramebuffersP, glDeleteRenderbuffersP = glProc("glDeleteFramebuffers"), glProc("glDeleteRenderbuffers")
	}
	delOne(glDeleteFramebuffersP, e.msFBO)
	delOne(glDeleteFramebuffersP, e.resolveFBO)
	delOne(glDeleteFramebuffersP, e.depthFBO)
	delOne(glDeleteRenderbuffersP, e.colorRB)
	delOne(glDeleteRenderbuffersP, e.depthRB)
	delOne(glDeleteTexturesP, e.tex)
	delOne(glDeleteTexturesP, e.depthTex)
}

func (t rtex) free() {
	if glDeleteFramebuffersP == 0 {
		glDeleteFramebuffersP, glDeleteRenderbuffersP = glProc("glDeleteFramebuffers"), glProc("glDeleteRenderbuffers")
	}
	delOne(glDeleteFramebuffersP, t.fbo)
	delOne(glDeleteTexturesP, t.tex)
}

// newEyeTargetBest: the requested MSAA level, falling back if the driver refuses it
func newEyeTargetBest(w, h int32, samples int) (*eyeTarget, error) {
	var err error
	for _, s := range []int{samples, 4, 2, 0} {
		if s > samples {
			continue
		}
		var t *eyeTarget
		if t, err = newEyeTarget(w, h, s); err == nil {
			if s != samples {
				log.Printf("eye target: fell back to %dx MSAA", s)
			}
			return t, nil
		}
		log.Printf("eye target %dx MSAA failed: %v", s, err)
	}
	return nil, err
}

func genOne(p uintptr) uint32 {
	call(p, 1, uintptr(unsafe.Pointer(&gIDs[0])))
	return gIDs[0]
}

func newEyeTarget(w, h int32, samples int) (*eyeTarget, error) {
	e := &eyeTarget{w: w, h: h}
	if glRenderbufferStorageMultisampleP == 0 {
		samples = 0
	}
	e.msFBO = genOne(glGenFramebuffersP)
	call(glBindFramebufferP, glFramebuffer, uintptr(e.msFBO))
	color := genOne(glGenRenderbuffersP)
	e.colorRB = color
	call(glBindRenderbufferP, glRenderbuffer, uintptr(color))
	if samples > 0 {
		call(glRenderbufferStorageMultisampleP, glRenderbuffer, uintptr(samples), 0x881A, uintptr(w), uintptr(h))
	} else {
		call(glRenderbufferStorageP, glRenderbuffer, 0x881A, uintptr(w), uintptr(h))
	}
	call(glFramebufferRenderbufferP, glFramebuffer, glColorAttachment0, glRenderbuffer, uintptr(color))
	depth := genOne(glGenRenderbuffersP)
	e.depthRB = depth
	call(glBindRenderbufferP, glRenderbuffer, uintptr(depth))
	if samples > 0 {
		call(glRenderbufferStorageMultisampleP, glRenderbuffer, uintptr(samples), glDepthComponent24, uintptr(w), uintptr(h))
	} else {
		call(glRenderbufferStorageP, glRenderbuffer, glDepthComponent24, uintptr(w), uintptr(h))
	}
	call(glFramebufferRenderbufferP, glFramebuffer, glDepthAttachment, glRenderbuffer, uintptr(depth))
	if s := uint32(call(glCheckFramebufferStatusP, glFramebuffer)); s != glFramebufferOK {
		return nil, fmt.Errorf("render target incomplete (0x%x)", s)
	}
	e.resolveFBO = genOne(glGenFramebuffersP)
	call(glBindFramebufferP, glFramebuffer, uintptr(e.resolveFBO))
	call(glGenTexturesP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
	e.tex = gIDs[0]
	call(glBindTextureP, glTexture2D, uintptr(e.tex))
	call(glTexParameteriP, glTexture2D, glTexMinFilter, glLinear)
	call(glTexParameteriP, glTexture2D, glTexMagFilter, glLinear)
	call(glTexParameteriP, glTexture2D, glTexMaxLevel, 0)
	call(glTexParameteriP, glTexture2D, glTexWrapS, glClampToEdge)
	call(glTexParameteriP, glTexture2D, glTexWrapT, glClampToEdge)
	call(glTexImage2DP, glTexture2D, 0, 0x881A, uintptr(w), uintptr(h), 0, glRGBA, 0x140B, 0)
	call(glFramebufferTexture2DP, glFramebuffer, glColorAttachment0, glTexture2D, uintptr(e.tex), 0)
	if s := uint32(call(glCheckFramebufferStatusP, glFramebuffer)); s != glFramebufferOK {
		return nil, fmt.Errorf("resolve target incomplete (0x%x)", s)
	}
	e.depthTex = newTex(w, h, glDepthComponent24, 0x1902, 0x1405, 0x2600)
	e.depthFBO = genOne(glGenFramebuffersP)
	call(glBindFramebufferP, glFramebuffer, uintptr(e.depthFBO))
	call(glFramebufferTexture2DP, glFramebuffer, glDepthAttachment, glTexture2D, uintptr(e.depthTex), 0)
	call(opengl32.NewProc("glDrawBuffer").Addr(), 0)
	call(opengl32.NewProc("glReadBuffer").Addr(), 0)
	if s := uint32(call(glCheckFramebufferStatusP, glFramebuffer)); s != glFramebufferOK {
		return nil, fmt.Errorf("depth target incomplete (0x%x)", s)
	}
	call(glBindFramebufferP, glFramebuffer, 0)
	return e, nil
}

func (e *eyeTarget) begin() {
	call(glBindFramebufferP, glFramebuffer, uintptr(e.msFBO))
	call(glViewportP, 0, 0, uintptr(e.w), uintptr(e.h))
}

func (e *eyeTarget) resolve() {
	call(glBindFramebufferP, glReadFramebuffer, uintptr(e.msFBO))
	call(glBindFramebufferP, glDrawFramebuffer, uintptr(e.resolveFBO))
	call(glBlitFramebufferP, 0, 0, uintptr(e.w), uintptr(e.h), 0, 0, uintptr(e.w), uintptr(e.h), glColorBufferBit, 0x2600)
	call(glBindFramebufferP, glFramebuffer, 0)
}

func (e *eyeTarget) mirror(ww, wh int32) {
	if ww <= 0 || wh <= 0 {
		return
	}
	call(glBindFramebufferP, glReadFramebuffer, uintptr(e.resolveFBO))
	call(glBindFramebufferP, glDrawFramebuffer, 0)
	call(glViewportP, 0, 0, uintptr(ww), uintptr(wh))
	call(glClearColorP, f32(0), f32(0), f32(0), f32(1))
	call(glClearP, glColorBufferBit)
	sw, sh := float32(e.w), float32(e.h)
	scale := float32(ww) / sw
	if s2 := float32(wh) / sh; s2 > scale {
		scale = s2
	}
	cw, ch := float32(ww)/scale, float32(wh)/scale
	x0, y0 := (sw-cw)/2, (sh-ch)/2
	call(glBlitFramebufferP, uintptr(int32(x0)), uintptr(int32(y0)), uintptr(int32(x0+cw)), uintptr(int32(y0+ch)), 0, 0, uintptr(ww), uintptr(wh), glColorBufferBit, glLinear)
	call(glBindFramebufferP, glFramebuffer, 0)
}

func loadMatrix(mode uintptr, m mat4) {
	*gMat = m
	call(glMatrixModeP, mode)
	call(glLoadMatrixfP, uintptr(unsafe.Pointer(gMat)))
}

func (b *batch) draw(mode uintptr) {
	n := len(b.v) / 7
	if n == 0 {
		return
	}
	call(glEnableClientStateP, glVertexArray)
	call(glEnableClientStateP, glColorArray)
	call(glVertexPointerP, 3, glFloat, 28, uintptr(unsafe.Pointer(&b.v[0])))
	call(glColorPointerP, 4, glFloat, 28, uintptr(unsafe.Pointer(&b.v[3])))
	call(glDrawArraysP, mode, 0, uintptr(n))
	call(glDisableClientStateP, glColorArray)
	call(glDisableClientStateP, glVertexArray)
}

func initGlowTex() {
	const n = 64
	px := glowTexture(n)
	call(glGenTexturesP, 1, uintptr(unsafe.Pointer(&gIDs[0])))
	glowTex = gIDs[0]
	call(glBindTextureP, glTexture2D, uintptr(glowTex))
	call(glTexParameteriP, glTexture2D, glTexMinFilter, glLinear)
	call(glTexParameteriP, glTexture2D, glTexMagFilter, glLinear)
	call(glTexParameteriP, glTexture2D, glTexMaxLevel, 0)
	call(glTexParameteriP, glTexture2D, glTexWrapS, glClampToEdge)
	call(glTexParameteriP, glTexture2D, glTexWrapT, glClampToEdge)
	call(glTexImage2DP, glTexture2D, 0, glRGBA8, n, n, 0, glRGBA, glUnsignedByte, uintptr(unsafe.Pointer(&px[0])))
	call(glBindTextureP, glTexture2D, 0)
}

func setFog(on bool, density float32, r, g, b float32) {
	if !on {
		call(glDisableP, glFog)
		return
	}
	call(glEnableP, glFog)
	call(glFogiP, glFogMode, glExp2)
	call(glFogfP, glFogDensity, f32(density))
	*fogCol = [4]float32{r, g, b, 1}
	call(glFogfvP, glFogColor, uintptr(unsafe.Pointer(fogCol)))
}

func (b *tbatch) draw() {
	n := len(b.v) / 9
	if n == 0 {
		return
	}
	if glowTex == 0 {
		initGlowTex()
	}
	call(glEnableP, glTexture2D)
	call(glBindTextureP, glTexture2D, uintptr(glowTex))
	call(glEnableClientStateP, glVertexArray)
	call(glEnableClientStateP, glColorArray)
	call(glEnableClientStateP, glTexCoordArray)
	call(glVertexPointerP, 3, glFloat, 36, uintptr(unsafe.Pointer(&b.v[0])))
	call(glTexCoordPointerP, 2, glFloat, 36, uintptr(unsafe.Pointer(&b.v[3])))
	call(glColorPointerP, 4, glFloat, 36, uintptr(unsafe.Pointer(&b.v[5])))
	call(glDrawArraysP, glTriangles, 0, uintptr(n))
	call(glDisableClientStateP, glTexCoordArray)
	call(glDisableClientStateP, glColorArray)
	call(glDisableClientStateP, glVertexArray)
	call(glBindTextureP, glTexture2D, 0)
	call(glDisableP, glTexture2D)
}
