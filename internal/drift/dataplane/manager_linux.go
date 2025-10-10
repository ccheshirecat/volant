<<<<<<< HEAD
<<<<<<< HEAD
//go:build linux

package dataplane

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type manager struct {
	logger   *slog.Logger
	program  *ebpf.Program
	portmap  *ebpf.Map
	link     link.Link
	iface    string
	mu       sync.Mutex
	closed   bool
	programs *ebpf.Collection
}

func newManager(opts Options) (Interface, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ObjectPath == "" {
		return nil, fmt.Errorf("dataplane: bpf object path required")
	}
	if opts.Interface == "" {
		return nil, fmt.Errorf("dataplane: interface name required")
	}

	spec, err := ebpf.LoadCollectionSpec(opts.ObjectPath)
	if err != nil {
		return nil, fmt.Errorf("dataplane: load collection spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("dataplane: create collection: %w", err)
	}

	prog, ok := coll.Programs["drift_l4_ingress"]
	if !ok {
		coll.Close()
		return nil, errors.New("dataplane: program drift_l4_ingress not found")
	}

	portmap, ok := coll.Maps["portmap"]
	if !ok {
		coll.Close()
		return nil, errors.New("dataplane: portmap not found")
	}

	iface, err := net.InterfaceByName(opts.Interface)
	if err != nil {
		coll.Close()
		return nil, fmt.Errorf("dataplane: lookup interface %s: %w", opts.Interface, err)
	}

	l, err := link.AttachTCX(link.TCXOptions{
		Program:   prog,
		Interface: iface.Index,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err != nil {
		coll.Close()
		return nil, fmt.Errorf("dataplane: attach tcx: %w", err)
	}

	return &manager{
		logger:   opts.Logger.With("component", "dataplane"),
		program:  prog,
		portmap:  portmap,
		link:     l,
		iface:    opts.Interface,
		programs: coll,
	}, nil
}

func (m *manager) ApplyBridge(_ context.Context, proto uint8, hostPort uint16, destIP net.IP, destPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("dataplane: manager closed")
	}

	ip4 := destIP.To4()
	if ip4 == nil {
		return fmt.Errorf("dataplane: destination ip %s not ipv4", destIP)
	}

	key := portmapKey{
		Proto: proto,
		Port:  htons(hostPort),
	}

	value := portmapValue{
		DestIP:   binary.BigEndian.Uint32(ip4),
		DestPort: htons(destPort),
	}

	if err := m.portmap.Put(&key, &value); err != nil {
		return fmt.Errorf("dataplane: portmap update: %w", err)
	}

	m.logger.Info("route applied", "proto", protoName(proto), "host_port", hostPort, "dest_ip", destIP.String(), "dest_port", destPort)
	return nil
}

func (m *manager) Remove(_ context.Context, proto uint8, hostPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("dataplane: manager closed")
	}

	key := portmapKey{
		Proto: proto,
		Port:  htons(hostPort),
	}

	if err := m.portmap.Delete(&key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("dataplane: portmap delete: %w", err)
	}

	m.logger.Info("route removed", "proto", protoName(proto), "host_port", hostPort)
	return nil
}

func (m *manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.link != nil {
		_ = m.link.Close()
	}
	if m.programs != nil {
		m.programs.Close()
	}
	return nil
}

type portmapKey struct {
	Proto uint8
	_     uint8
	Port  uint16
}

type portmapValue struct {
	DestIP   uint32
	DestPort uint16
	_        uint16
}

func htons(value uint16) uint16 {
	return value<<8 | value>>8
}

func protoName(proto uint8) string {
	switch proto {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("%d", proto)
	}
}
||||||| parent of 1682feb (Add Drift L4 switch integration)
=======
||||||| parent of 3d2c157 (fix bpf headers)
<<<<<<< HEAD
//go:build linux

package dataplane

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type manager struct {
	logger   *slog.Logger
	program  *ebpf.Program
	portmap  *ebpf.Map
	link     link.Link
	iface    string
	mu       sync.Mutex
	closed   bool
	programs *ebpf.Collection
}

func newManager(opts Options) (Interface, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ObjectPath == "" {
		return nil, fmt.Errorf("dataplane: bpf object path required")
	}
	if opts.Interface == "" {
		return nil, fmt.Errorf("dataplane: interface name required")
	}

	spec, err := ebpf.LoadCollectionSpec(opts.ObjectPath)
	if err != nil {
		return nil, fmt.Errorf("dataplane: load collection spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("dataplane: create collection: %w", err)
	}

	prog, ok := coll.Programs["drift_l4_ingress"]
	if !ok {
		coll.Close()
		return nil, errors.New("dataplane: program drift_l4_ingress not found")
	}

	portmap, ok := coll.Maps["portmap"]
	if !ok {
		coll.Close()
		return nil, errors.New("dataplane: portmap not found")
	}

	iface, err := net.InterfaceByName(opts.Interface)
	if err != nil {
		coll.Close()
		return nil, fmt.Errorf("dataplane: lookup interface %s: %w", opts.Interface, err)
	}

	l, err := link.AttachTC(link.TCOptions{
		Program:     prog,
		Interface:   iface.Index,
		AttachPoint: link.TCIngress,
	})
	if err != nil {
		coll.Close()
		return nil, fmt.Errorf("dataplane: attach tc: %w", err)
	}

	return &manager{
		logger:   opts.Logger.With("component", "dataplane"),
		program:  prog,
		portmap:  portmap,
		link:     l,
		iface:    opts.Interface,
		programs: coll,
	}, nil
}

func (m *manager) ApplyBridge(_ context.Context, proto uint8, hostPort uint16, destIP net.IP, destPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("dataplane: manager closed")
	}

	ip4 := destIP.To4()
	if ip4 == nil {
		return fmt.Errorf("dataplane: destination ip %s not ipv4", destIP)
	}

	key := portmapKey{
		Proto: proto,
		Port:  htons(hostPort),
	}

	value := portmapValue{
		DestIP:   binary.BigEndian.Uint32(ip4),
		DestPort: htons(destPort),
	}

	if err := m.portmap.Put(&key, &value); err != nil {
		return fmt.Errorf("dataplane: portmap update: %w", err)
	}

	m.logger.Info("route applied", "proto", protoName(proto), "host_port", hostPort, "dest_ip", destIP.String(), "dest_port", destPort)
	return nil
}

func (m *manager) Remove(_ context.Context, proto uint8, hostPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("dataplane: manager closed")
	}

	key := portmapKey{
		Proto: proto,
		Port:  htons(hostPort),
	}

	if err := m.portmap.Delete(&key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("dataplane: portmap delete: %w", err)
	}

	m.logger.Info("route removed", "proto", protoName(proto), "host_port", hostPort)
	return nil
}

func (m *manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.link != nil {
		_ = m.link.Close()
	}
	if m.programs != nil {
		m.programs.Close()
	}
	return nil
}

type portmapKey struct {
	Proto uint8
	_     uint8
	Port  uint16
}

type portmapValue struct {
	DestIP   uint32
	DestPort uint16
	_        uint16
}

func htons(value uint16) uint16 {
	return value<<8 | value>>8
}

func protoName(proto uint8) string {
	switch proto {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("%d", proto)
	}
}
||||||| parent of 1682feb (Add Drift L4 switch integration)
=======
=======
>>>>>>> 3d2c157 (fix bpf headers)
//go:build linux

package dataplane

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type manager struct {
	logger   *slog.Logger
	program  *ebpf.Program
	portmap  *ebpf.Map
	link     link.Link
	iface    string
	mu       sync.Mutex
	closed   bool
	programs *ebpf.Collection
}

func newManager(opts Options) (Interface, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ObjectPath == "" {
		return nil, fmt.Errorf("dataplane: bpf object path required")
	}
	if opts.Interface == "" {
		return nil, fmt.Errorf("dataplane: interface name required")
	}

	spec, err := ebpf.LoadCollectionSpec(opts.ObjectPath)
	if err != nil {
		return nil, fmt.Errorf("dataplane: load collection spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("dataplane: create collection: %w", err)
	}

	prog, ok := coll.Programs["drift_l4_ingress"]
	if !ok {
		coll.Close()
		return nil, errors.New("dataplane: program drift_l4_ingress not found")
	}

	portmap, ok := coll.Maps["portmap"]
	if !ok {
		coll.Close()
		return nil, errors.New("dataplane: portmap not found")
	}

	iface, err := net.InterfaceByName(opts.Interface)
	if err != nil {
		coll.Close()
		return nil, fmt.Errorf("dataplane: lookup interface %s: %w", opts.Interface, err)
	}

	l, err := link.AttachTCX(link.TCXOptions{
		Program:   prog,
		Interface: iface.Index,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err != nil {
		coll.Close()
		return nil, fmt.Errorf("dataplane: attach tcx: %w", err)
	}

	return &manager{
		logger:   opts.Logger.With("component", "dataplane"),
		program:  prog,
		portmap:  portmap,
		link:     l,
		iface:    opts.Interface,
		programs: coll,
	}, nil
}

func (m *manager) ApplyBridge(_ context.Context, proto uint8, hostPort uint16, destIP net.IP, destPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("dataplane: manager closed")
	}

	ip4 := destIP.To4()
	if ip4 == nil {
		return fmt.Errorf("dataplane: destination ip %s not ipv4", destIP)
	}

	key := portmapKey{
		Proto: proto,
		Port:  htons(hostPort),
	}

	value := portmapValue{
		DestIP:   binary.BigEndian.Uint32(ip4),
		DestPort: htons(destPort),
	}

	if err := m.portmap.Put(&key, &value); err != nil {
		return fmt.Errorf("dataplane: portmap update: %w", err)
	}

	m.logger.Info("route applied", "proto", protoName(proto), "host_port", hostPort, "dest_ip", destIP.String(), "dest_port", destPort)
	return nil
}

func (m *manager) Remove(_ context.Context, proto uint8, hostPort uint16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("dataplane: manager closed")
	}

	key := portmapKey{
		Proto: proto,
		Port:  htons(hostPort),
	}

	if err := m.portmap.Delete(&key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("dataplane: portmap delete: %w", err)
	}

	m.logger.Info("route removed", "proto", protoName(proto), "host_port", hostPort)
	return nil
}

func (m *manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.link != nil {
		_ = m.link.Close()
	}
	if m.programs != nil {
		m.programs.Close()
	}
	return nil
}

type portmapKey struct {
	Proto uint8
	_     uint8
	Port  uint16
}

type portmapValue struct {
	DestIP   uint32
	DestPort uint16
	_        uint16
}

func htons(value uint16) uint16 {
	return value<<8 | value>>8
}

func protoName(proto uint8) string {
	switch proto {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("%d", proto)
	}
}
