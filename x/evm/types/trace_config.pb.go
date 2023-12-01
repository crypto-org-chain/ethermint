// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: ethermint/evm/v1/trace_config.proto

package types

import (
	fmt "fmt"
	_ "github.com/cosmos/gogoproto/gogoproto"
	proto "github.com/cosmos/gogoproto/proto"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

// TraceConfig holds extra parameters to trace functions.
type TraceConfig struct {
	// tracer is a custom javascript tracer
	Tracer string `protobuf:"bytes,1,opt,name=tracer,proto3" json:"tracer,omitempty"`
	// timeout overrides the default timeout of 5 seconds for JavaScript-based tracing
	// calls
	Timeout string `protobuf:"bytes,2,opt,name=timeout,proto3" json:"timeout,omitempty"`
	// reexec defines the number of blocks the tracer is willing to go back
	Reexec uint64 `protobuf:"varint,3,opt,name=reexec,proto3" json:"reexec,omitempty"`
	// disable_stack switches stack capture
	DisableStack bool `protobuf:"varint,5,opt,name=disable_stack,json=disableStack,proto3" json:"disableStack"`
	// disable_storage switches storage capture
	DisableStorage bool `protobuf:"varint,6,opt,name=disable_storage,json=disableStorage,proto3" json:"disableStorage"`
	// debug can be used to print output during capture end
	Debug bool `protobuf:"varint,8,opt,name=debug,proto3" json:"debug,omitempty"`
	// limit defines the maximum length of output, but zero means unlimited
	Limit int32 `protobuf:"varint,9,opt,name=limit,proto3" json:"limit,omitempty"`
	// overrides can be used to execute a trace using future fork rules
	Overrides *ChainConfig `protobuf:"bytes,10,opt,name=overrides,proto3" json:"overrides,omitempty"`
	// enable_memory switches memory capture
	EnableMemory bool `protobuf:"varint,11,opt,name=enable_memory,json=enableMemory,proto3" json:"enableMemory"`
	// enable_return_data switches the capture of return data
	EnableReturnData bool `protobuf:"varint,12,opt,name=enable_return_data,json=enableReturnData,proto3" json:"enableReturnData"`
	// tracer_config configures the tracer config
	TracerConfig *TracerConfig `protobuf:"bytes,13,opt,name=tracer_config,json=tracerConfig,proto3" json:"tracerConfig"`
}

func (m *TraceConfig) Reset()         { *m = TraceConfig{} }
func (m *TraceConfig) String() string { return proto.CompactTextString(m) }
func (*TraceConfig) ProtoMessage()    {}
func (*TraceConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_8f7fb70914ae5a53, []int{0}
}
func (m *TraceConfig) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *TraceConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_TraceConfig.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *TraceConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TraceConfig.Merge(m, src)
}
func (m *TraceConfig) XXX_Size() int {
	return m.Size()
}
func (m *TraceConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_TraceConfig.DiscardUnknown(m)
}

var xxx_messageInfo_TraceConfig proto.InternalMessageInfo

func (m *TraceConfig) GetTracer() string {
	if m != nil {
		return m.Tracer
	}
	return ""
}

func (m *TraceConfig) GetTimeout() string {
	if m != nil {
		return m.Timeout
	}
	return ""
}

func (m *TraceConfig) GetReexec() uint64 {
	if m != nil {
		return m.Reexec
	}
	return 0
}

func (m *TraceConfig) GetDisableStack() bool {
	if m != nil {
		return m.DisableStack
	}
	return false
}

func (m *TraceConfig) GetDisableStorage() bool {
	if m != nil {
		return m.DisableStorage
	}
	return false
}

func (m *TraceConfig) GetDebug() bool {
	if m != nil {
		return m.Debug
	}
	return false
}

func (m *TraceConfig) GetLimit() int32 {
	if m != nil {
		return m.Limit
	}
	return 0
}

func (m *TraceConfig) GetOverrides() *ChainConfig {
	if m != nil {
		return m.Overrides
	}
	return nil
}

func (m *TraceConfig) GetEnableMemory() bool {
	if m != nil {
		return m.EnableMemory
	}
	return false
}

func (m *TraceConfig) GetEnableReturnData() bool {
	if m != nil {
		return m.EnableReturnData
	}
	return false
}

func (m *TraceConfig) GetTracerConfig() *TracerConfig {
	if m != nil {
		return m.TracerConfig
	}
	return nil
}

// TracerConfig holds extra parameters for configuring the call tracer.
type TracerConfig struct {
	// If true, call tracer won't collect any subcalls
	OnlyTopCall bool `protobuf:"varint,1,opt,name=only_top_call,json=onlyTopCall,proto3" json:"onlyTopCall"`
	// If true, call tracer will collect event logs
	WithLog bool `protobuf:"varint,2,opt,name=with_log,json=withLog,proto3" json:"withLog"`
}

func (m *TracerConfig) Reset()         { *m = TracerConfig{} }
func (m *TracerConfig) String() string { return proto.CompactTextString(m) }
func (*TracerConfig) ProtoMessage()    {}
func (*TracerConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_8f7fb70914ae5a53, []int{1}
}
func (m *TracerConfig) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *TracerConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_TracerConfig.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *TracerConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TracerConfig.Merge(m, src)
}
func (m *TracerConfig) XXX_Size() int {
	return m.Size()
}
func (m *TracerConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_TracerConfig.DiscardUnknown(m)
}

var xxx_messageInfo_TracerConfig proto.InternalMessageInfo

func (m *TracerConfig) GetOnlyTopCall() bool {
	if m != nil {
		return m.OnlyTopCall
	}
	return false
}

func (m *TracerConfig) GetWithLog() bool {
	if m != nil {
		return m.WithLog
	}
	return false
}

func init() {
	proto.RegisterType((*TraceConfig)(nil), "ethermint.evm.v1.TraceConfig")
	proto.RegisterType((*TracerConfig)(nil), "ethermint.evm.v1.TracerConfig")
}

func init() {
	proto.RegisterFile("ethermint/evm/v1/trace_config.proto", fileDescriptor_8f7fb70914ae5a53)
}

var fileDescriptor_8f7fb70914ae5a53 = []byte{
	// 499 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x64, 0x52, 0xbb, 0x6e, 0xdb, 0x30,
	0x14, 0x8d, 0x5a, 0x3f, 0x64, 0xda, 0x6e, 0x0c, 0xd6, 0x28, 0x88, 0x00, 0x95, 0x84, 0x14, 0x08,
	0x34, 0x49, 0x48, 0x83, 0x4e, 0x59, 0x0a, 0xb9, 0x53, 0xd0, 0x2e, 0x6c, 0xba, 0x74, 0x11, 0x68,
	0x99, 0x95, 0x89, 0x48, 0xa2, 0x41, 0xd1, 0x6a, 0xfc, 0x17, 0xfd, 0x9b, 0xfe, 0x42, 0xc7, 0x8c,
	0x9d, 0x84, 0xc2, 0xde, 0xf4, 0x15, 0x05, 0x29, 0xf9, 0x15, 0x6f, 0x3c, 0xe7, 0xdc, 0x4b, 0x1e,
	0xde, 0x7b, 0xc0, 0x3b, 0x2a, 0xe7, 0x54, 0xa4, 0x2c, 0x93, 0x3e, 0x2d, 0x52, 0xbf, 0xb8, 0xf6,
	0xa5, 0x20, 0x11, 0x0d, 0x23, 0x9e, 0xfd, 0x60, 0xb1, 0xb7, 0x10, 0x5c, 0x72, 0x38, 0xda, 0x15,
	0x79, 0xb4, 0x48, 0xbd, 0xe2, 0xfa, 0x62, 0x1c, 0xf3, 0x98, 0x6b, 0xd1, 0x57, 0xa7, 0xba, 0xee,
	0xe2, 0xf4, 0xb2, 0x68, 0x4e, 0x58, 0x76, 0x74, 0xd9, 0xe5, 0xef, 0x16, 0xe8, 0xdf, 0xab, 0x37,
	0x26, 0x9a, 0x85, 0x6f, 0x40, 0x47, 0x3f, 0x29, 0x90, 0xe1, 0x18, 0x6e, 0x0f, 0x37, 0x08, 0x22,
	0xd0, 0x95, 0x2c, 0xa5, 0x7c, 0x29, 0xd1, 0x0b, 0x2d, 0x6c, 0xa1, 0xea, 0x10, 0x94, 0x3e, 0xd2,
	0x08, 0xbd, 0x74, 0x0c, 0xb7, 0x85, 0x1b, 0x04, 0x3f, 0x80, 0xe1, 0x8c, 0xe5, 0x64, 0x9a, 0xd0,
	0x30, 0x97, 0x24, 0x7a, 0x40, 0x6d, 0xc7, 0x70, 0xcd, 0x60, 0x54, 0x95, 0xf6, 0xa0, 0x11, 0xbe,
	0x2a, 0x1e, 0x1f, 0x21, 0x78, 0x0b, 0xce, 0xf7, 0x6d, 0x5c, 0x90, 0x98, 0xa2, 0x8e, 0x6e, 0x84,
	0x55, 0x69, 0xbf, 0xda, 0x95, 0x6a, 0x05, 0x3f, 0xc3, 0x70, 0x0c, 0xda, 0x33, 0x3a, 0x5d, 0xc6,
	0xc8, 0x54, 0x2d, 0xb8, 0x06, 0x8a, 0x4d, 0x58, 0xca, 0x24, 0xea, 0x39, 0x86, 0xdb, 0xc6, 0x35,
	0x80, 0xb7, 0xa0, 0xc7, 0x0b, 0x2a, 0x04, 0x9b, 0xd1, 0x1c, 0x01, 0xc7, 0x70, 0xfb, 0xef, 0xdf,
	0x7a, 0xcf, 0x47, 0xeb, 0x4d, 0xd4, 0xc8, 0xea, 0xd9, 0xe0, 0x7d, 0xbd, 0xfa, 0x1c, 0xcd, 0xb4,
	0xc9, 0x94, 0xa6, 0x5c, 0xac, 0x50, 0x7f, 0xff, 0xb9, 0x5a, 0xf8, 0xa2, 0x79, 0x7c, 0x84, 0x60,
	0x00, 0x60, 0xd3, 0x26, 0xa8, 0x5c, 0x8a, 0x2c, 0x9c, 0x11, 0x49, 0xd0, 0x40, 0xf7, 0x8e, 0xab,
	0xd2, 0x1e, 0xd5, 0x2a, 0xd6, 0xe2, 0x27, 0x22, 0x09, 0x3e, 0x61, 0xe0, 0x37, 0x30, 0xac, 0x77,
	0xd2, 0x2c, 0x12, 0x0d, 0xb5, 0x77, 0xeb, 0xd4, 0xbb, 0xde, 0xab, 0xa8, 0xcd, 0xd7, 0xd6, 0xe4,
	0x01, 0x83, 0x8f, 0xd0, 0x5d, 0xcb, 0x6c, 0x8d, 0xda, 0x77, 0x2d, 0xb3, 0x3b, 0x32, 0x77, 0x63,
	0x6d, 0x3e, 0x87, 0x5f, 0x6f, 0xf1, 0x81, 0xeb, 0xcb, 0x07, 0x30, 0x38, 0x7c, 0x00, 0xde, 0x80,
	0x21, 0xcf, 0x92, 0x55, 0x28, 0xf9, 0x22, 0x8c, 0x48, 0x92, 0xe8, 0x00, 0x99, 0xc1, 0x79, 0x55,
	0xda, 0x7d, 0x25, 0xdc, 0xf3, 0xc5, 0x84, 0x24, 0x09, 0x3e, 0x04, 0xf0, 0x0a, 0x98, 0x3f, 0x99,
	0x9c, 0x87, 0x09, 0x8f, 0x75, 0xae, 0xcc, 0xa0, 0x5f, 0x95, 0x76, 0x57, 0x71, 0x9f, 0x79, 0x8c,
	0xb7, 0x87, 0xe0, 0xe3, 0x9f, 0xb5, 0x65, 0x3c, 0xad, 0x2d, 0xe3, 0xdf, 0xda, 0x32, 0x7e, 0x6d,
	0xac, 0xb3, 0xa7, 0x8d, 0x75, 0xf6, 0x77, 0x63, 0x9d, 0x7d, 0xbf, 0x8a, 0x99, 0x9c, 0x2f, 0xa7,
	0x5e, 0xc4, 0x53, 0x15, 0x73, 0x9e, 0xfb, 0xfb, 0xd8, 0x3f, 0xea, 0xe0, 0xcb, 0xd5, 0x82, 0xe6,
	0xd3, 0x8e, 0xce, 0xfb, 0xcd, 0xff, 0x00, 0x00, 0x00, 0xff, 0xff, 0x44, 0x9c, 0xe7, 0x2e, 0x63,
	0x03, 0x00, 0x00,
}

func (m *TraceConfig) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *TraceConfig) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *TraceConfig) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.TracerConfig != nil {
		{
			size, err := m.TracerConfig.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintTraceConfig(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x6a
	}
	if m.EnableReturnData {
		i--
		if m.EnableReturnData {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x60
	}
	if m.EnableMemory {
		i--
		if m.EnableMemory {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x58
	}
	if m.Overrides != nil {
		{
			size, err := m.Overrides.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintTraceConfig(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x52
	}
	if m.Limit != 0 {
		i = encodeVarintTraceConfig(dAtA, i, uint64(m.Limit))
		i--
		dAtA[i] = 0x48
	}
	if m.Debug {
		i--
		if m.Debug {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x40
	}
	if m.DisableStorage {
		i--
		if m.DisableStorage {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x30
	}
	if m.DisableStack {
		i--
		if m.DisableStack {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x28
	}
	if m.Reexec != 0 {
		i = encodeVarintTraceConfig(dAtA, i, uint64(m.Reexec))
		i--
		dAtA[i] = 0x18
	}
	if len(m.Timeout) > 0 {
		i -= len(m.Timeout)
		copy(dAtA[i:], m.Timeout)
		i = encodeVarintTraceConfig(dAtA, i, uint64(len(m.Timeout)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.Tracer) > 0 {
		i -= len(m.Tracer)
		copy(dAtA[i:], m.Tracer)
		i = encodeVarintTraceConfig(dAtA, i, uint64(len(m.Tracer)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *TracerConfig) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *TracerConfig) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *TracerConfig) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.WithLog {
		i--
		if m.WithLog {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x10
	}
	if m.OnlyTopCall {
		i--
		if m.OnlyTopCall {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintTraceConfig(dAtA []byte, offset int, v uint64) int {
	offset -= sovTraceConfig(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *TraceConfig) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Tracer)
	if l > 0 {
		n += 1 + l + sovTraceConfig(uint64(l))
	}
	l = len(m.Timeout)
	if l > 0 {
		n += 1 + l + sovTraceConfig(uint64(l))
	}
	if m.Reexec != 0 {
		n += 1 + sovTraceConfig(uint64(m.Reexec))
	}
	if m.DisableStack {
		n += 2
	}
	if m.DisableStorage {
		n += 2
	}
	if m.Debug {
		n += 2
	}
	if m.Limit != 0 {
		n += 1 + sovTraceConfig(uint64(m.Limit))
	}
	if m.Overrides != nil {
		l = m.Overrides.Size()
		n += 1 + l + sovTraceConfig(uint64(l))
	}
	if m.EnableMemory {
		n += 2
	}
	if m.EnableReturnData {
		n += 2
	}
	if m.TracerConfig != nil {
		l = m.TracerConfig.Size()
		n += 1 + l + sovTraceConfig(uint64(l))
	}
	return n
}

func (m *TracerConfig) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.OnlyTopCall {
		n += 2
	}
	if m.WithLog {
		n += 2
	}
	return n
}

func sovTraceConfig(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozTraceConfig(x uint64) (n int) {
	return sovTraceConfig(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *TraceConfig) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTraceConfig
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: TraceConfig: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: TraceConfig: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Tracer", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTraceConfig
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthTraceConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Tracer = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Timeout", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTraceConfig
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthTraceConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Timeout = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Reexec", wireType)
			}
			m.Reexec = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Reexec |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field DisableStack", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.DisableStack = bool(v != 0)
		case 6:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field DisableStorage", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.DisableStorage = bool(v != 0)
		case 8:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Debug", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.Debug = bool(v != 0)
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Limit", wireType)
			}
			m.Limit = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Limit |= int32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 10:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Overrides", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthTraceConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthTraceConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Overrides == nil {
				m.Overrides = &ChainConfig{}
			}
			if err := m.Overrides.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 11:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field EnableMemory", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.EnableMemory = bool(v != 0)
		case 12:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field EnableReturnData", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.EnableReturnData = bool(v != 0)
		case 13:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field TracerConfig", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthTraceConfig
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthTraceConfig
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.TracerConfig == nil {
				m.TracerConfig = &TracerConfig{}
			}
			if err := m.TracerConfig.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTraceConfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTraceConfig
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *TracerConfig) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTraceConfig
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: TracerConfig: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: TracerConfig: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field OnlyTopCall", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.OnlyTopCall = bool(v != 0)
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field WithLog", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.WithLog = bool(v != 0)
		default:
			iNdEx = preIndex
			skippy, err := skipTraceConfig(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTraceConfig
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipTraceConfig(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowTraceConfig
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowTraceConfig
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthTraceConfig
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupTraceConfig
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthTraceConfig
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthTraceConfig        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowTraceConfig          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupTraceConfig = fmt.Errorf("proto: unexpected end of group")
)
