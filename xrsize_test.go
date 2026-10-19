package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"
)

func TestXRLayout(t *testing.T) {
	got := map[string]uintptr{
		"XrInstanceCreateInfo": unsafe.Sizeof(xrInstanceCreateInfo{}), "XrInstanceCreateInfo.enabledExtensionNames": unsafe.Offsetof(xrInstanceCreateInfo{}.extNames),
		"XrSystemGetInfo": unsafe.Sizeof(xrSystemGetInfo{}), "XrSessionCreateInfo": unsafe.Sizeof(xrSessionCreateInfo{}), "XrSessionBeginInfo": unsafe.Sizeof(xrSessionBeginInfo{}),
		"XrReferenceSpaceCreateInfo": unsafe.Sizeof(xrReferenceSpaceCreateInfo{}), "XrReferenceSpaceCreateInfo.poseInReferenceSpace": unsafe.Offsetof(xrReferenceSpaceCreateInfo{}.pose),
		"XrViewConfigurationView": unsafe.Sizeof(xrViewConfigView{}), "XrSwapchainCreateInfo": unsafe.Sizeof(xrSwapchainCreateInfo{}), "XrSwapchainCreateInfo.mipCount": unsafe.Offsetof(xrSwapchainCreateInfo{}.mipCount),
		"XrSwapchainImageWaitInfo": unsafe.Sizeof(xrSwapchainWaitInfo{}), "XrFrameState": unsafe.Sizeof(xrFrameState{}), "XrFrameState.shouldRender": unsafe.Offsetof(xrFrameState{}.shouldRender),
		"XrView": unsafe.Sizeof(xrView{}), "XrViewLocateInfo": unsafe.Sizeof(xrViewLocateInfo{}), "XrViewLocateInfo.space": unsafe.Offsetof(xrViewLocateInfo{}.space), "XrViewState": unsafe.Sizeof(xrViewState{}),
		"XrSwapchainSubImage": unsafe.Sizeof(xrSubImage{}), "XrCompositionLayerProjectionView": unsafe.Sizeof(xrProjView{}), "XrCompositionLayerProjectionView.subImage": unsafe.Offsetof(xrProjView{}.sub),
		"XrCompositionLayerProjection": unsafe.Sizeof(xrLayerProjection{}), "XrCompositionLayerProjection.views": unsafe.Offsetof(xrLayerProjection{}.views),
		"XrFrameEndInfo": unsafe.Sizeof(xrFrameEndInfo{}), "XrFrameEndInfo.layers": unsafe.Offsetof(xrFrameEndInfo{}.layers), "XrEventDataBuffer": unsafe.Sizeof(xrEventBuffer{}),
		"XrEventDataSessionStateChanged.state": unsafe.Offsetof(xrEventSessionState{}.state), "XrActionSetCreateInfo": unsafe.Sizeof(xrActionSetCreateInfo{}), "XrActionCreateInfo": unsafe.Sizeof(xrActionCreateInfo{}),
		"XrActionCreateInfo.localizedActionName": unsafe.Offsetof(xrActionCreateInfo{}.localized), "XrActionSuggestedBinding": unsafe.Sizeof(xrSuggestedBinding{}),
		"XrInteractionProfileSuggestedBinding": unsafe.Sizeof(xrProfileBindings{}), "XrSessionActionSetsAttachInfo": unsafe.Sizeof(xrAttachInfo{}), "XrActionSpaceCreateInfo": unsafe.Sizeof(xrActionSpaceCreateInfo{}),
		"XrActionSpaceCreateInfo.poseInActionSpace": unsafe.Offsetof(xrActionSpaceCreateInfo{}.pose), "XrActiveActionSet": unsafe.Sizeof(xrActiveActionSet{}), "XrActionsSyncInfo": unsafe.Sizeof(xrActionsSyncInfo{}),
		"XrActionStateGetInfo": unsafe.Sizeof(xrActionStateGetInfo{}), "XrActionStateBoolean": unsafe.Sizeof(xrActionStateBool{}), "XrActionStateBoolean.isActive": unsafe.Offsetof(xrActionStateBool{}.active),
		"XrSpaceLocation": unsafe.Sizeof(xrSpaceLocation{}), "XrSpaceLocation.pose": unsafe.Offsetof(xrSpaceLocation{}.pose), "XrHapticVibration": unsafe.Sizeof(xrHapticVibration{}), "XrHapticVibration.amplitude": unsafe.Offsetof(xrHapticVibration{}.amplitude),
	}
	if os.Getenv("XR_C_SIZES") == "" {
		t.Skip("set XR_C_SIZES to the C layout dump")
	}
	b, _ := os.ReadFile(os.Getenv("XR_C_SIZES"))
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var k string
		var v uintptr
		fmt.Sscan(l, &k, &v)
		g, ok := got[k]
		if !ok {
			t.Errorf("missing %s", k)
			continue
		}
		if g != v {
			t.Errorf("%s: go %d c %d", k, g, v)
		}
		n++
	}
	if n != len(got) {
		t.Errorf("checked %d of %d", n, len(got))
	}
}
