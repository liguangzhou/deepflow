/*
 * Copyright (c) 2022 Yunshan Networks
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package decoder

import (
	"time"
	"unsafe"

	"github.com/gogo/protobuf/proto"
	logging "github.com/op/go-logging"

	"github.com/deepflowys/deepflow/message/trident"
	ingestercommon "github.com/deepflowys/deepflow/server/ingester/common"
	"github.com/deepflowys/deepflow/server/ingester/pcap/config"
	"github.com/deepflowys/deepflow/server/ingester/pcap/dbwriter"
	"github.com/deepflowys/deepflow/server/libs/codec"
	"github.com/deepflowys/deepflow/server/libs/queue"
	"github.com/deepflowys/deepflow/server/libs/receiver"
	"github.com/deepflowys/deepflow/server/libs/stats"
	"github.com/deepflowys/deepflow/server/libs/utils"
)

var log = logging.MustGetLogger("pcap.decoder")

const (
	BUFFER_SIZE = 1024
)

type PcapHeader struct {
	Magic    uint32
	Major    uint16
	Minor    uint16
	ThisZone uint32
	SigFigs  uint32
	SnapLen  uint32
	LinkType uint32
}

type Counter struct {
	InCount    int64 `statsd:"in-count"`
	OutCount   int64 `statsd:"out-count"`
	ErrorCount int64 `statsd:"err-count"`
}

type Decoder struct {
	inQueue    queue.QueueReader
	pcapWriter *dbwriter.PcapWriter
	config     *config.Config

	counter *Counter
	utils.Closable
}

func NewDecoder(
	inQueue queue.QueueReader,
	pcapWriter *dbwriter.PcapWriter,
	config *config.Config,
) *Decoder {
	return &Decoder{
		inQueue:    inQueue,
		pcapWriter: pcapWriter,
		config:     config,
		counter:    &Counter{},
	}
}

func (d *Decoder) GetCounter() interface{} {
	var counter *Counter
	counter, d.counter = d.counter, &Counter{}
	return counter
}

func (d *Decoder) Run() {
	log.Infof("pcap decoder run")
	ingestercommon.RegisterCountableForIngester("decoder", d, stats.OptionStatTags{
		"msg_type": "pcap"})
	decoder := &codec.SimpleDecoder{}
	pcapBatch := &trident.PcapBatch{}
	pcapHeader := &PcapHeader{
		Major:    0x0200,
		Minor:    0x0400,
		ThisZone: 0, // GMT
		SigFigs:  0,
		SnapLen:  65535,
		LinkType: 1, // Ethernet
	}
	buffer := make([]interface{}, BUFFER_SIZE)
	for {
		n := d.inQueue.Gets(buffer)
		for i := 0; i < n; i++ {
			if buffer[i] == nil {
				continue
			}
			d.counter.InCount++
			recvBytes, ok := buffer[i].(*receiver.RecvBuffer)
			if !ok {
				log.Warning("get decode queue data type wrong")
				continue
			}
			decoder.Init(recvBytes.Buffer[recvBytes.Begin:recvBytes.End])
			d.handlePcap(recvBytes.VtapID, decoder, pcapHeader, pcapBatch)
			receiver.ReleaseRecvBuffer(recvBytes)
		}
	}
}

func (d *Decoder) handlePcap(vtapID uint16, decoder *codec.SimpleDecoder, pcapHeader *PcapHeader, pcapBatch *trident.PcapBatch) {
	var err error
	for !decoder.IsEnd() {
		bytes := decoder.ReadBytes()
		if len(bytes) > 0 {
			err = proto.Unmarshal(bytes, pcapBatch)
		}
		if decoder.Failed() || err != nil {
			if d.counter.ErrorCount == 0 {
				log.Errorf("OpenTelemetry log decode failed, offset=%d len=%d err: %s", decoder.Offset(), len(decoder.Bytes()), err)
			}
			d.counter.ErrorCount++
			return
		}
		pcapHeader.Magic = pcapBatch.GetMagic()
		for _, pcap := range pcapBatch.Batches {
			d.counter.OutCount++
			d.pcapWriter.Write(pcapToStore(vtapID, pcapHeader, pcap))
		}
	}
}

func pcapToStore(vtapID uint16, pcapHeader *PcapHeader, pcap *trident.Pcap) *dbwriter.PcapStore {
	s := dbwriter.AcquirePcapStore()
	s.Time = uint32(time.Duration(pcap.GetStartTime()) / time.Second)
	s.EndTime = int64(pcap.GetEndTime())
	s.VtapID = vtapID
	s.FlowID = pcap.GetFlowId()
	s.PacketCount = pcap.GetPacketCount()
	s.PacketBatch = append(s.PacketBatch[:0], (*(*[unsafe.Sizeof(pcapHeader)]byte)(unsafe.Pointer(&pcapHeader)))[:]...)
	s.PacketBatch = append(s.PacketBatch, pcap.GetPacketRecords()...)
	return s
}
