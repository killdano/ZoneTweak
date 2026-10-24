package main

type xrQuat struct{ x, y, z, w float32 }
type xrVec3f struct{ x, y, z float32 }
type xrPose struct {
	o xrQuat
	p xrVec3f
}
type xrFov struct{ l, r, u, d float32 }

type xrApplicationInfo struct {
	appName       [128]byte
	appVersion    uint32
	engineName    [128]byte
	engineVersion uint32
	apiVersion    uint64
}

const xrTypeInstanceProperties = 32

type xrInstanceProperties struct {
	typ            int32
	next           uintptr
	runtimeVersion uint64
	runtimeName    [128]byte
}

type xrInstanceCreateInfo struct {
	typ         int32
	next        uintptr
	createFlags uint64
	app         xrApplicationInfo
	layerCount  uint32
	layerNames  uintptr
	extCount    uint32
	extNames    uintptr
}

type xrSystemGetInfo struct {
	typ        int32
	next       uintptr
	formFactor int32
}

type xrGraphicsRequirementsOpenGL struct {
	typ            int32
	next           uintptr
	minApi, maxApi uint64
}

type xrGraphicsBindingOpenGLWin32 struct {
	typ        int32
	next       uintptr
	hDC, hGLRC uintptr
}

type xrSessionCreateInfo struct {
	typ         int32
	next        uintptr
	createFlags uint64
	systemId    uint64
}

type xrSessionBeginInfo struct {
	typ        int32
	next       uintptr
	viewConfig int32
}

type xrReferenceSpaceCreateInfo struct {
	typ     int32
	next    uintptr
	refType int32
	pose    xrPose
}

type xrViewConfigView struct {
	typ                                int32
	next                               uintptr
	recW, maxW, recH, maxH, recS, maxS uint32
}

type xrSwapchainCreateInfo struct {
	typ                                                        int32
	next                                                       uintptr
	createFlags                                                uint64
	usage                                                      uint64
	format                                                     int64
	sampleCount, width, height, faceCount, arraySize, mipCount uint32
}

type xrSwapchainImageGL struct {
	typ   int32
	next  uintptr
	image uint32
}

type xrPlainInfo struct {
	typ  int32
	next uintptr
}

type xrSwapchainWaitInfo struct {
	typ     int32
	next    uintptr
	timeout int64
}

type xrFrameState struct {
	typ          int32
	next         uintptr
	displayTime  int64
	period       int64
	shouldRender uint32
}

type xrView struct {
	typ  int32
	next uintptr
	pose xrPose
	fov  xrFov
}

type xrViewLocateInfo struct {
	typ        int32
	next       uintptr
	viewConfig int32
	time       int64
	space      uintptr
}

type xrViewState struct {
	typ   int32
	next  uintptr
	flags uint64
}

type xrSubImage struct {
	swapchain  uintptr
	x, y, w, h int32
	arrayIndex uint32
}

type xrProjView struct {
	typ  int32
	next uintptr
	pose xrPose
	fov  xrFov
	sub  xrSubImage
}

type xrLayerProjection struct {
	typ       int32
	next      uintptr
	flags     uint64
	space     uintptr
	viewCount uint32
	views     uintptr
}

type xrFrameEndInfo struct {
	typ         int32
	next        uintptr
	displayTime int64
	blend       int32
	layerCount  uint32
	layers      uintptr
}

type xrEventBuffer struct {
	typ     int32
	next    uintptr
	varying [4000]byte
}

type xrEventSessionState struct {
	typ     int32
	next    uintptr
	session uintptr
	state   int32
	time    int64
}

type xrActionSetCreateInfo struct {
	typ       int32
	next      uintptr
	name      [64]byte
	localized [128]byte
	priority  uint32
}

type xrActionCreateInfo struct {
	typ        int32
	next       uintptr
	name       [64]byte
	actionType int32
	subCount   uint32
	subPaths   uintptr
	localized  [128]byte
}

type xrSuggestedBinding struct {
	action  uintptr
	binding uint64
}

type xrProfileBindings struct {
	typ      int32
	next     uintptr
	profile  uint64
	count    uint32
	bindings uintptr
}

type xrAttachInfo struct {
	typ   int32
	next  uintptr
	count uint32
	sets  uintptr
}

type xrActionSpaceCreateInfo struct {
	typ     int32
	next    uintptr
	action  uintptr
	subPath uint64
	pose    xrPose
}

type xrActiveActionSet struct {
	set     uintptr
	subPath uint64
}

type xrActionsSyncInfo struct {
	typ   int32
	next  uintptr
	count uint32
	sets  uintptr
}

type xrActionStateGetInfo struct {
	typ     int32
	next    uintptr
	action  uintptr
	subPath uint64
}

type xrActionStateBool struct {
	typ        int32
	next       uintptr
	current    uint32
	changed    uint32
	lastChange int64
	active     uint32
}

type xrActionStateFloat struct {
	typ        int32
	next       uintptr
	current    float32
	changed    uint32
	lastChange int64
	active     uint32
}

type xrSpaceLocation struct {
	typ   int32
	next  uintptr
	flags uint64
	pose  xrPose
}

type xrHapticVibration struct {
	typ       int32
	next      uintptr
	duration  int64
	frequency float32
	amplitude float32
}

func poseTo34(p xrPose) hmdMatrix34 {
	x, y, z, w := p.o.x, p.o.y, p.o.z, p.o.w
	return hmdMatrix34{
		{1 - 2*(y*y+z*z), 2 * (x*y - z*w), 2 * (x*z + y*w), p.p.x},
		{2 * (x*y + z*w), 1 - 2*(x*x+z*z), 2 * (y*z - x*w), p.p.y},
		{2 * (x*z - y*w), 2 * (y*z + x*w), 1 - 2*(x*x+y*y), p.p.z},
	}
}

type xrEventSpaceChange struct {
	typ        int32
	next       uintptr
	session    uintptr
	refType    int32
	changeTime int64
	poseValid  uint32
	pose       xrPose
}
