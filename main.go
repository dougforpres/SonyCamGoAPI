package main

import (
	"Sony/Web/winstruct"
	"bytes"
	"fmt"
	"github.com/go-ole/go-ole"
	"net/http"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/gin-gonic/gin"
)

var (
	cameraDLL                  = syscall.NewLazyDLL("SonyMTPCamera.dll")
	procGetCameraInfo          = cameraDLL.NewProc("GetCameraInfo")
	procGetDeviceInfo          = cameraDLL.NewProc("GetDeviceInfo")
	procGetPropertyDescriptor  = cameraDLL.NewProc("GetPropertyDescriptor")
	procGetPropertyValueOption = cameraDLL.NewProc("GetPropertyValueOption")
	//	procGetSinglePropertyValue = cameraDLL.NewProc("GetSinglePropertyValue")
	//	procRefreshPropertyList    = cameraDLL.NewProc("RefreshPropertyList")
	procCloseDevice            = cameraDLL.NewProc("CloseDevice")
	procGetPortableDeviceCount = cameraDLL.NewProc("GetPortableDeviceCount")
	procGetPortableDeviceInfo  = cameraDLL.NewProc("GetPortableDeviceInfo")
	procOpenDeviceEx           = cameraDLL.NewProc("OpenDeviceEx")
	procGetPropertyList        = cameraDLL.NewProc("GetPropertyList")
	procGetAllPropertyValues   = cameraDLL.NewProc("GetAllPropertyValues")
	procGetPreviewImage        = cameraDLL.NewProc("GetPreviewImage")
)

type emptyResponse struct{}

type cameraHandle struct {
	Handle uint64 `json:"handle"`
}

type openJson struct {
	ID string `json:"id"`
}

// device contains basic info about an enumerated camera
// the "devicePath" value is used when the camera is opened
type device struct {
	ID           string `json:"id" windows:"LPWSTR"`
	Manufacturer string `json:"manufacturer" windows:"LPWSTR"`
	Model        string `json:"model" windows:"LPWSTR"`
	RegistryPath string `json:"registryPath" windows:"LPWSTR"`
}

type imageMeta struct {
	SensorImageWidth   uint32 `json:"sensorImageWidth"`
	SensorImageHeight  uint32 `json:"sensorImageHeight"`
	CroppedImageWidth  uint32 `json:"croppedImageWidth"`
	CroppedImageHeight uint32 `json:"croppedImageHeight"`
	PreviewWidth       uint32 `json:"previewWidth"`
	PreviewHeight      uint32 `json:"previewHeight"`
	CropMode           string `json:"cropMode"`
}

type pixelMeta struct {
	PixelWidth   float64 `json:"pixelWidth"`
	PixelHeight  float64 `json:"pixelHeight"`
	BitsPerPixel uint32  `json:"bitsPerPixel"`
}

type deviceMeta struct {
	Manufacturer  string `json:"manufacturer"`
	Model         string `json:"model"`
	SerialNumber  string `json:"serialNumber"`
	DeviceName    string `json:"deviceName"`
	SensorName    string `json:"sensorName"`
	DeviceVersion string `json:"deviceVersion"`
}

type CameraInfo struct {
	Size   imageMeta  `json:"size"`
	Pixel  pixelMeta  `json:"pixel"`
	Device deviceMeta `json:"device"`
}

type deviceInfo struct {
	Version            uint32  `json:"version" windows:"DWORD"`            // 0
	SensorImageWidth   uint32  `json:"sensorImageWidth" windows:"DWORD"`   // 4
	SensorImageHeight  uint32  `json:"sensorImageHeight" windows:"DWORD"`  // 8
	CroppedImageWidth  uint32  `json:"croppedImageWidth" windows:"DWORD"`  // 12
	CroppedImageHeight uint32  `json:"croppedImageHeight" windows:"DWORD"` // 16
	BayerXOffset       uint32  `json:"bayerXOffset" windows:"DWORD"`       // 20
	BayerYOffset       uint32  `json:"bayerYOffset" windows:"DWORD"`       // 24
	CropMode           uint32  `json:"cropMode" windows:"DWORD"`           // 28
	ExposureTimeMin    float64 `json:"-" windows:"double"`                 // 32
	ExposureTimeMax    float64 `json:"-" windows:"double"`                 // 40
	ExposureTimeStep   float64 `json:"-" windows:"double"`                 // 48
	PixelWidth         float64 `json:"pixelWidth" windows:"double"`        // 56
	PixelHeight        float64 `json:"pixelHeight" windows:"double"`       // 64
	BitsPerPixel       uint32  `json:"bitsPerPixel" windows:"DWORD"`       // 72
	BppPad             uint32  `json:"-" windows:"DWORD"`                  // 76 - needed due to 8-byte padding for pointers
	Manufacturer       string  `json:"manufacturer" windows:"LPWSTR"`      // 80
	Model              string  `json:"model" windows:"LPWSTR"`             // 88
	SerialNumber       string  `json:"serialNumber" windows:"LPWSTR"`      // 96
	DeviceName         string  `json:"deviceName" windows:"LPWSTR"`        // 104
	SensorName         string  `json:"sensorName" windows:"LPWSTR"`        // 112
	DeviceVersion      string  `json:"deviceVersion" windows:"LPWSTR"`     // 120 > 127
}

// cameraInfo contains info about a camera - much of this is the same as
// deviceInfo, however this is an in/out structure that is filled in when the
// camera is being learnt
type camera struct {
	Flags              uint32  `json:"flags" windows:"DWORD"`
	SensorImageWidth   uint32  `json:"sensorImageWidth" windows:"DWORD"`
	SensorImageHeight  uint32  `json:"sensorImageHeight" windows:"DWORD"`
	CroppedImageWidth  uint32  `json:"croppedImageWidth" windows:"DWORD"`
	CroppedImageHeight uint32  `json:"croppedImageHeight" windows:"DWORD"`
	PreviewWidth       uint32  `json:"previewWidth" windows:"DWORD"`
	PreviewHeight      uint32  `json:"previewHeight" windows:"DWORD"`
	BayerXOffset       uint32  `json:"bayerXOffset" windows:"DWORD"`
	BayerYOffset       uint32  `json:"bayerYOffset" windows:"DWORD"`
	Pad                uint32  `json:"-" windows:"DWORD"` // 76 - needed due to 8-byte padding for pointers
	PixelWidth         float64 `json:"pixelWidth" windows:"double"`
	PixelHeight        float64 `json:"pixelHeight" windows:"double"`
}

// propertyValueOption contains enum values for enum type properties
type propertyValueOption struct {
	Value    uint   `json:"value" windows:"DWORD"`
	ValuePad uint   `json:"-" windows:"DWORD"`
	Name     string `json:"name" windows:"LPWSTR"`
}

// propertyValue contains a property value
type propertyValue struct {
	ID    uint   `json:"id" windows:"DWORD"`
	Value uint   `json:"value" windows:"DWORD"`
	Text  string `json:"text" windows:"LPWSTR"`
}

type propertyDescriptor struct {
	ID         uint                  `json:"id" windows:"DWORD"`
	TypeId     uint                  `json:"-" windows:"WORD"`
	Type       string                `json:"type" windows:"-"`
	Flags      uint                  `json:"flags" windows:"WORD"`
	Name       string                `json:"name" windows:"LPWSTR"`
	ValueCount uint                  `json:"-" windows:"DWORD"`
	Values     []propertyValueOption `json:"enum,omitempty" windows:"-"`
}

type imageInfo struct {
	Size      uint   `json:"size" windows:"DWORD"`
	SizePad   uint   `json:"-" windows:"DWORD"`
	Data      []byte `json:"data" windows:"LPBYTE,Size"`
	Status    uint   `json:"status" windows:"DWORD"`
	ImageMode uint   `json:"imageMode" windows:"DWORD"`
	Width     uint   `json:"width" windows:"DWORD"`
	Height    uint   `json:"height" windows:"DWORD"`
	Flags     uint   `json:"flags" windows:"DWORD"`
	//	FlagsPad  uint    `json:"-" windows:"DWORD"`
	MetaSize uint    `json:"metaSize" windows:"DWORD"`
	Meta     []byte  `json:"meta" windows:"LPBYTE,MetaSize"`
	Duration float64 `json:"duration" windows:"double"`
}

func CameraDLL() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Because each request can be on a different thread, we need to initialize com
		// but we can't uninitialize at the end or code using com will lose all its refs
		// so if initialize "fails" it (in general) means it's already been done on this
		// thread, so we uninitialize to prevent runaway ref counts
		//		fmt.Printf("Initializing COM\n")
		err := ole.CoInitialize(0)

		if err != nil {
			ole.CoUninitialize()
		}

		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func main() {
	fmt.Printf("Initializing COM\n")
	err := ole.CoInitialize(0)

	if err != nil {
		fmt.Printf("Got error calling coinitialize: %s", err)
	}

	router := gin.Default()
	router.Use(CORSMiddleware())
	router.Use(CameraDLL())

	router.GET("/devices", getDevices)
	router.GET("/cameras/:handle/info", getDeviceInfo)
	router.POST("/cameras", openCamera)
	router.DELETE("/cameras/:handle", closeCamera)
	router.GET("/cameras/:handle/propertyDescriptors", getCameraPropertyDescriptors)
	router.GET("/cameras/:handle/properties", getCameraProperties)
	router.GET("/cameras/:handle/preview", getPreviewImage)

	_ = router.Run("localhost:8080")

	ole.CoUninitialize()
}

// getDevices returns a list of devices that are recognized by Windows as cameras
func getDevices(c *gin.Context) {
	// Try to call DLL to get list
	count, _, _ := procGetPortableDeviceCount.Call()
	deviceCount := int(count)
	var devices []device

	for index := 0; index < deviceCount; index++ {
		var d device
		b := winstruct.NewByteBuffer(&d)
		_, _, _ = procGetPortableDeviceInfo.Call(uintptr(index), getPointerToSlice(b.Bytes()))
		winstruct.Unmarshal(b, &d)
		devices = append(devices, d)
	}

	c.IndentedJSON(http.StatusOK, devices)
}

func getPointerToSlice(buffer []byte) uintptr {
	// I wanted to use unsafe.SliceData, but it has a funky type that was getting in the way
	// the "&buffer[:1][0]" is directly from its documentation:
	// 		SliceData returns a pointer to the underlying array of the argument
	// 		slice.
	//   		- If cap(slice) > 0, SliceData returns &slice[:1][0].
	//   		- If slice == nil, SliceData returns nil.
	//   		- Otherwise, SliceData returns a non-nil pointer to an
	//     		  unspecified memory address.

	return uintptr(unsafe.Pointer(&buffer[:1][0]))
}

func getCameraHandleFromPath(c *gin.Context) uintptr {
	var hC, _ = strconv.Atoi(c.Param("handle"))
	return uintptr(hC)
}

func cropModeAsString(cropMode uint32) string {
	switch cropMode {
	case 0:
		return "none"
	case 1:
		return "auto"
	case 2:
		return "user"
	default:
		return "unknown"
	}
}

func typeIdToString(id uint) string {
	switch id {
	case 0x0000:
		return "unknown"
	case 0x0001:
		return "int8"
	case 0x0002:
		return "uint8"
	case 0x0003:
		return "int16"
	case 0x0004:
		return "uint16"
	case 0x0005:
		return "int32"
	case 0x0006:
		return "uint32"
	case 0x0007:
		return "int64"
	case 0x0008:
		return "uint64"
	case 0x0009:
		return "int128"
	case 0x0010:
		return "uint128"
	case 0xa001:
		return "int8[]"
	case 0xa002:
		return "uint8[]"
	case 0xa003:
		return "int16[]"
	case 0xa004:
		return "uint16[]"
	case 0xa005:
		return "int32[]"
	case 0xa006:
		return "uint32[]"
	case 0xa007:
		return "int64[]"
	case 0xa008:
		return "uint64[]"
	case 0xa009:
		return "int128[]"
	case 0xa010:
		return "uint128[]"
	case 0xffff:
		return "string"
	default:
		return fmt.Sprintf("Unknown format x%04x", id)
	}
}

func getDeviceInfo(c *gin.Context) {
	// This method actually uses data from DeviceInfo and CameraInfo to generate the response
	// Each contains some different data - and eventually I'd like to combine them
	hCamera := getCameraHandleFromPath(c)

	dInfo := deviceInfo{Version: 1}
	buffer := winstruct.Marshal(&dInfo)
	_, _, _ = procGetDeviceInfo.Call(hCamera, getPointerToSlice(buffer.Bytes()))
	winstruct.Unmarshal(buffer, &dInfo)

	cInfo := camera{}
	buffer = winstruct.NewByteBuffer(&cInfo)
	_, _, _ = procGetCameraInfo.Call(hCamera, getPointerToSlice(buffer.Bytes()))
	winstruct.Unmarshal(buffer, &cInfo)

	// Construct resultant output
	info := CameraInfo{
		Size: imageMeta{
			SensorImageWidth:   cInfo.SensorImageWidth,
			SensorImageHeight:  cInfo.SensorImageHeight,
			CroppedImageWidth:  cInfo.CroppedImageWidth,
			CroppedImageHeight: cInfo.CroppedImageHeight,
			PreviewWidth:       cInfo.PreviewWidth,
			PreviewHeight:      cInfo.PreviewHeight,
			CropMode:           cropModeAsString(dInfo.CropMode),
		},
		Pixel: pixelMeta{
			PixelWidth:   cInfo.PixelWidth,
			PixelHeight:  cInfo.PixelHeight,
			BitsPerPixel: dInfo.BitsPerPixel,
		},
		Device: deviceMeta{
			Manufacturer:  dInfo.Manufacturer,
			Model:         dInfo.Model,
			SerialNumber:  dInfo.SerialNumber,
			DeviceVersion: dInfo.DeviceVersion,
			SensorName:    dInfo.SensorName,
			DeviceName:    dInfo.DeviceName,
		},
	}

	c.IndentedJSON(http.StatusOK, info)
}

func openCamera(c *gin.Context) {
	in := openJson{}
	_ = c.ShouldBindJSON(&in)
	var hCamera uintptr

	sptr, _ := syscall.UTF16PtrFromString(in.ID)
	hCamera, _, _ = procOpenDeviceEx.Call(uintptr(unsafe.Pointer(sptr)), 0)

	result := cameraHandle{Handle: uint64(hCamera)}
	c.IndentedJSON(http.StatusOK, result)
}

func closeCamera(c *gin.Context) {
	hCamera := getCameraHandleFromPath(c)

	_, _, _ = procCloseDevice.Call(hCamera)

	c.IndentedJSON(http.StatusOK, emptyResponse{})
}

func getCameraPropertyDescriptors(c *gin.Context) {
	hCamera := getCameraHandleFromPath(c)

	count := 0

	hr, _, _ := procGetPropertyList.Call(hCamera, 0, uintptr(unsafe.Pointer(&count)))
	var descriptors []propertyDescriptor

	if hr == 1237 { //windows.ERROR_RETRY
		b := make([]byte, count*4)

		hr, _, _ = procGetPropertyList.Call(hCamera, getPointerToSlice(b), uintptr(unsafe.Pointer(&count)))

		for i := 0; i < count; i++ {
			offs := i * 4
			id := uint(b[offs]) + uint(b[offs+1])<<8 + uint(b[offs+2])<<16 + uint(b[offs+3])<<24

			// Get Property Descriptor
			pd := propertyDescriptor{}
			buffer := winstruct.NewByteBuffer(&pd)
			hr, _, _ = procGetPropertyDescriptor.Call(hCamera, uintptr(id), getPointerToSlice(buffer.Bytes()))
			winstruct.Unmarshal(buffer, &pd)
			pd.Type = typeIdToString(pd.TypeId)

			// Get options (if this is an enum)
			if pd.ValueCount > 0 {
				var option propertyValueOption

				for j := uint(0); j < pd.ValueCount; j++ {
					optionBuffer := winstruct.NewByteBuffer(&option)
					_, _, _ = procGetPropertyValueOption.Call(hCamera, uintptr(id), getPointerToSlice(optionBuffer.Bytes()), uintptr(j))

					winstruct.Unmarshal(optionBuffer, &option)
					pd.Values = append(pd.Values, option)
				}
			}

			descriptors = append(descriptors, pd)
		}
	}

	c.IndentedJSON(http.StatusOK, descriptors)
}

func getCameraProperties(c *gin.Context) {
	hCamera := getCameraHandleFromPath(c)

	// Driver will tend to return cached info to keep fast performance
	//	_, _, _ = procRefreshPropertyList.Call(hCamera)

	count := 0
	_, _, _ = procGetAllPropertyValues.Call(hCamera, 0, uintptr(unsafe.Pointer(&count)))

	if count == 0 {
		panic(fmt.Sprintf("Unable to get properties - seems there are none!"))
	}

	neededSize := winstruct.Size(&propertyValue{})
	b := make([]byte, neededSize*count)
	var properties []propertyValue

	_, _, _ = procGetAllPropertyValues.Call(hCamera, getPointerToSlice(b), uintptr(unsafe.Pointer(&count)))

	buffer := bytes.NewBuffer(b)

	for i := 0; i < count; i++ {
		var property propertyValue

		winstruct.Unmarshal(buffer, &property)
		properties = append(properties, property)
	}

	c.IndentedJSON(http.StatusOK, properties)
}

func getPreviewImage(c *gin.Context) {
	hCamera := getCameraHandleFromPath(c)

	info := imageInfo{ImageMode: 3} // JPEG
	buffer := winstruct.Marshal(&info)

	_, _, _ = procGetPreviewImage.Call(hCamera, getPointerToSlice(buffer.Bytes()))

	winstruct.Unmarshal(buffer, &info)

	c.IndentedJSON(http.StatusOK, info)
}
