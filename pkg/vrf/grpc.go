// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package vrf is the main package of the application
package vrf

import (
	"context"
	"fmt"
	"log"
	"reflect"

	"github.com/dgraph-io/badger"
	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/resourceid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
)

// CreateVrf executes the creation of the VRF
func (s *Server) CreateVrf(ctx context.Context, in *pb.CreateVrfRequest) (*pb.Vrf, error) {
	// check input correctness
	if err := s.validateCreateVrfRequest(in); err != nil {
		fmt.Printf("CreateVrf(): validation failure: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.VrfId != "" {
		log.Printf("CreateVrf(): client provided the ID of a resource %v, ignoring the name field %v", in.VrfId, in.Vrf.Name)
		resourceID = in.VrfId
	}
	in.Vrf.Name = resourceIDToFullName(resourceID)
	// idempotent API when called with same key, should return same object
	vrfObj, err := s.getVrf(in.Vrf.Name)
	if err != nil {
		if err != infradb.ErrKeyNotFound {
			fmt.Printf("CreateVrf(): Failed to interact with store: %v", err)
			return nil, err
		}
	} else {
		log.Printf("CreateVrf(): Already existing Vrf with id %v", in.Vrf.Name)
		return vrfObj, nil
	}

	// Store the domain object into DB
	response, err := s.createVrf(in.Vrf)
	if err != nil {
		log.Printf("CreateVrf(): Vrf with id %v, Create Vrf to DB failure: %v", in.Vrf.Name, err)
		return nil, err
	}
	return response, nil
}

// DeleteVrf deletes a VRF
func (s *Server) DeleteVrf(ctx context.Context, in *pb.DeleteVrfRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteVrfRequest(in); err != nil {
		fmt.Printf("DeleteVrf(): validation failure: %v", err)
		return nil, err
	}
	// fetch object from the database
	_, err := s.getVrf(in.Name)
	if err != nil {
		if err != badger.ErrKeyNotFound {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		if !in.AllowMissing {
			err = status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
			fmt.Printf("DeleteVrf(): Vrf with id %v: Not Found %v", in.Name, err)
			return nil, err
		}
		return &emptypb.Empty{}, nil
	}

	if err := s.deleteVrf(in.Name); err != nil {
		log.Printf("DeleteVrf(): Vrf with id %v, Delete Vrf from DB failure: %v", in.Name, err)
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// UpdateVrf updates an VRF
func (s *Server) UpdateVrf(ctx context.Context, in *pb.UpdateVrfRequest) (*pb.Vrf, error) {
	// check input correctness
	if err := s.validateUpdateVrfRequest(in); err != nil {
		fmt.Printf("UpdateVrf(): validation failure: %v", err)
		return nil, err
	}
	// fetch object from the database
	vrfObj, err := s.getVrf(in.Vrf.Name)
	if err != nil {
		if err != infradb.ErrKeyNotFound {
			fmt.Printf("UpdateVrf(): Failed to interact with store: %v", err)
			return nil, err
		}
		if !in.AllowMissing {
			err = status.Errorf(codes.NotFound, "unable to find key %s", in.Vrf.Name)
			fmt.Printf("UpdateVrf(): Vrf with id %v: Not Found %v", in.Vrf.Name, err)
			return nil, err
		}

		log.Printf("UpdateVrf(): Vrf with id %v is not found so it will be created", in.Vrf.Name)

		// Store the domain object into DB
		response, err := s.createVrf(in.Vrf)
		if err != nil {
			log.Printf("UpdateVrf(): Vrf with id %v, Create Vrf to DB failure: %v", in.Vrf.Name, err)
			return nil, err
		}
		return response, nil
	}
	// We do that because we need to see if the object before and after the application of the mask is equal.
	// If it is the we just return the old object.
	updatedvrfObj := utils.ProtoClone(vrfObj)

	//Apply updateMask to the current Pb object
	utils.ApplyMaskToStoredPbObject(in.UpdateMask, updatedvrfObj, in.Vrf)

	// Check if the object before the application of the field mask
	// is different with the one after the application of the field mask
	if reflect.DeepEqual(vrfObj, updatedvrfObj) {
		return vrfObj, nil
	}

	response, err := s.updateVrf(updatedvrfObj)
	if err != nil {
		log.Printf("UpdateVrf(): Vrf with id %v, Update Vrf to DB failure: %v", in.Vrf.Name, err)
		return nil, err
	}

	return response, nil
}

// GetVrf gets an VRF
func (s *Server) GetVrf(ctx context.Context, in *pb.GetVrfRequest) (*pb.Vrf, error) {
	// check input correctness
	if err := s.validateGetVrfRequest(in); err != nil {
		fmt.Printf("GetVrf(): validation failure: %v", err)
		return nil, err
	}
	// fetch object from the database
	vrfObj, err := s.getVrf(in.Name)
	if err != nil {
		if err != infradb.ErrKeyNotFound {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		err = status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		fmt.Printf("GetVrf(): Vrf with id %v: Not Found %v", in.Name, err)
		return nil, err
	}

	return vrfObj, nil
}

// ListVrfs lists logical bridges
func (s *Server) ListVrfs(_ context.Context, in *pb.ListVrfsRequest) (*pb.ListVrfsResponse, error) {
	// check required fields
	if err := s.validateListVrfsRequest(in); err != nil {
		fmt.Printf("ListVrfs(): validation failure: %v", err)
		return nil, err
	}
	// fetch pagination from the database, calculate size and offset
	size, offset, err := utils.ExtractPagination(in.PageSize, in.PageToken, s.Pagination)
	if err != nil {
		return nil, err
	}
	// fetch object from the database
	Blobarray := []*pb.Vrf{}
	// Dimitris: ListHelper is a  go map that helps on retrieving the objects
	// from DB by name. The reason that we need it is because the DB doesn't support any
	// List() function to retrieve all the VRF objects in one operation by using a prefix as key and not
	// the full name. The prefix can be: "//network.opiproject.org/vrfs"
	// In a replay scenario the List must be filled again as it will be out of sync with the DB status.
	for key := range s.ListHelper {
		vrfObj, err := s.getVrf(key)
		if err != nil {
			if err != badger.ErrKeyNotFound {
				fmt.Printf("Failed to interact with store: %v", err)
				return nil, err
			}
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			fmt.Printf("ListVrfs(): Vrf with id %v: Not Found %v", key, err)
			return nil, err
		}
		Blobarray = append(Blobarray, vrfObj)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sortVrfs(Blobarray)
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := utils.LimitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	return &pb.ListVrfsResponse{Vrfs: Blobarray, NextPageToken: token}, nil
}
