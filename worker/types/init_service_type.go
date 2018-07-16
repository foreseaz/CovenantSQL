/*
 * Copyright 2018 The ThunderDB Authors.
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

package types

import (
	"gitlab.com/thunderdb/ThunderDB/kayak"
	"gitlab.com/thunderdb/ThunderDB/proto"
	ct "gitlab.com/thunderdb/ThunderDB/sqlchain/types"
)

// InitService defines worker service init request.
type InitService struct {
	proto.Envelope
}

// ResourceMeta defines single database resource meta.
type ResourceMeta struct {
	Node   uint16 // reserved node count
	Space  uint64 // reserved storage space in bytes
	Memory uint64 // reserved memory in bytes
}

// ServiceInstance defines single instance to be initialized.
type ServiceInstance struct {
	DatabaseID   proto.DatabaseID
	Peers        *kayak.Peers
	ResourceMeta ResourceMeta
	GenesisBlock *ct.Block
}

// InitServiceResponse defines worker service init response.
type InitServiceResponse struct {
	Instances []ServiceInstance
}