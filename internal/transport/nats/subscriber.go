package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martin/network-service/internal/domain"
	"github.com/martin/network-service/internal/service"
	"github.com/martin/network-service/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const (
	SubjectUserRegistered = "dev.auth.v1.user.registered"
	SubjectVPCCreate      = "dev.network.v1.vpc.create"
	SubjectVPCCreated     = "dev.network.v1.vpc.created"
	SubjectVPCList        = "dev.network.v1.vpc.list"

	SubjectVPCDefaultGet     = "dev.network.v1.vpc.default.get"
	SubjectVPCValidate       = "dev.network.v1.vpc.validate"
	SubjectVPCDefaultResolve = "dev.network.v1.vpc.default.resolve"
	SubjectVPCReconcile      = "dev.network.v1.vpc.reconcile"

	SubjectResourceAttach = "dev.network.v1.resource.attach"
	SubjectResourceDetach = "dev.network.v1.resource.detach"

	SubjectResourceResolve  = "dev.network.v1.resource.resolve"
	SubjectVPCResourcesList = "dev.network.v1.vpc.resources.list"

	SubjectComputeRegister   = "dev.network.v1.compute.register"
	SubjectComputeHealth     = "dev.network.v1.compute.health"
	SubjectComputeDeregister = "dev.network.v1.compute.deregister"
	SubjectComputeList       = "dev.network.v1.compute.list"
	SubjectComputeRoute      = "dev.network.v1.compute.route"

	SubjectComputeLifecycle = "dev.compute.v1.instance.lifecycle"

	SubjectInstancePrepare = "dev.network.v1.instance.prepare"
	SubjectInstanceRelease = "dev.network.v1.instance.release"

	SubjectEIPAllocate     = "dev.network.v1.eip.allocate"
	SubjectEIPAssociate    = "dev.network.v1.eip.associate"
	SubjectEIPDisassociate = "dev.network.v1.eip.disassociate"

	SubjectRDSExpose   = "dev.network.v1.rds.expose"
	SubjectRDSUnexpose = "dev.network.v1.rds.unexpose"
)

type Subscriber struct {
	nc         *nats.Conn
	netService service.NetworkService
}

func NewSubscriber(nc *nats.Conn, netService service.NetworkService) *Subscriber {
	return &Subscriber{
		nc:         nc,
		netService: netService,
	}
}

func (s *Subscriber) Subscribe() error {
	logger.WithContext(context.Background()).Info("Subscribing to NATS subject", zap.String("subject", SubjectUserRegistered))

	_, err := s.nc.Subscribe(SubjectUserRegistered, func(msg *nats.Msg) {
		var event domain.UserRegisteredEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			logger.WithContext(context.Background()).Error("failed to unmarshal user registered event",
				zap.Error(err),
				zap.String("raw_payload", string(msg.Data)))
			return
		}

		// Extract or generate correlation ID
		correlationID := event.CorrelationID
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		// Inject into context
		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, correlationID)
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		l := logger.WithContext(ctx)

		l.Info("Received user registered event",
			zap.String("tenant_id", event.TenantID),
			zap.String("tenant_name", event.TenantName))

		if _, err := s.netService.CreateDefaultVPC(ctx, event.TenantID, event.TenantName); err != nil {
			l.Error("failed to create default VPC for tenant",
				zap.Error(err),
				zap.String("tenant_id", event.TenantID))
			return
		}

		l.Info("Successfully processed user registered event", zap.String("tenant_id", event.TenantID))
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectUserRegistered, err)
	}

	// 2. Custom VPC Create
	_, err = s.nc.Subscribe(SubjectVPCCreate, func(msg *nats.Msg) {
		var event domain.CreateVPCEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			logger.WithContext(context.Background()).Error("failed to unmarshal VPC create event",
				zap.Error(err),
				zap.String("raw_payload", string(msg.Data)))
			return
		}

		correlationID := event.CorrelationID
		if correlationID == "" {
			correlationID = uuid.New().String()
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, correlationID)
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second) // Networking can be slow
		defer cancel()

		l := logger.WithContext(ctx)
		l.Info("Received VPC create event",
			zap.String("tenant_id", event.TenantID),
			zap.String("vpc_name", event.VPCName))

		vpc, err := s.netService.CreateVPC(ctx, event.TenantID, event.VPCName)

		status := domain.VPCStatusActive
		vpcID := ""
		cidr := ""
		if err != nil {
			l.Error("Failed to create custom VPC", zap.Error(err))
			status = domain.VPCStatusError
		} else {
			vpcID = vpc.ID
			cidr = vpc.CIDRBlock
		}

		// Publish result event
		resp := domain.VPCCreatedEvent{
			CorrelationID: correlationID,
			TenantID:      event.TenantID,
			VPCID:         vpcID,
			CIDRBlock:     cidr,
			Status:        status,
		}

		data, _ := json.Marshal(resp)
		if err := s.nc.Publish(SubjectVPCCreated, data); err != nil {
			l.Error("Failed to publish VPC created event", zap.Error(err))
		} else {
			l.Info("Published VPC created event", zap.String("vpc_id", vpcID), zap.String("status", status))
		}
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectVPCCreate, err)
	}

	// 2b. List VPCs (Request-Reply)
	_, err = s.nc.Subscribe(SubjectVPCList, func(msg *nats.Msg) {
		var req domain.ListVPCsRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Listing VPCs", zap.String("tenant_id", req.TenantID))

		vpcs, err := s.netService.ListVPCs(ctx, req.TenantID)
		resp := domain.ListVPCsResponse{
			CorrelationID: req.CorrelationID,
			TenantID:      req.TenantID,
		}
		
		if vpcs == nil {
			resp.VPCs = []domain.VPC{}
		} else {
			resp.VPCs = vpcs
		}

		if err != nil {
			resp.Error = err.Error()
			l.Error("Failed to list VPCs", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectVPCList, err)
	}

	// 3. Resolve Default VPC (Request-Reply)
	_, err = s.nc.Subscribe(SubjectVPCDefaultGet, func(msg *nats.Msg) {
		var req domain.GetDefaultVPCRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Resolving default VPC", zap.String("tenant_id", req.TenantID))

		vpc, err := s.netService.GetDefaultVPC(ctx, req.TenantID)
		resp := domain.GetDefaultVPCResponse{
			CorrelationID: req.CorrelationID,
			TenantID:      req.TenantID,
		}

		if err == nil && vpc != nil {
			resp.VPCID = vpc.ID
			resp.Status = vpc.Status
			resp.BridgeName = vpc.BridgeName
			l.Info("Resolved default VPC for response", zap.String("vpc_id", vpc.ID), zap.String("bridge_name", vpc.BridgeName))
		} else if err != nil {
			l.Error("Failed to get default VPC", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 4. Validate VPC (Request-Reply)
	_, err = s.nc.Subscribe(SubjectVPCValidate, func(msg *nats.Msg) {
		var req domain.ValidateVPCRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Validating VPC ownership", zap.String("tenant_id", req.TenantID), zap.String("vpc_id", req.VPCID))

		valid, status, reason, err := s.netService.ValidateVPC(ctx, req.TenantID, req.VPCID)
		resp := domain.ValidateVPCResponse{
			CorrelationID: req.CorrelationID,
			Valid:         valid,
			Status:        status,
			Reason:        reason,
		}
		if err != nil {
			resp.Reason = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 5. Attach Resource to Subnet (Request-Reply)
	_, err = s.nc.Subscribe(SubjectResourceAttach, func(msg *nats.Msg) {
		var req domain.AttachResourceRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(
			zap.String("tenant_id", req.TenantID),
			zap.String("resource_arn", req.ResourceARN),
			zap.String("vpc_id", req.TargetVPCID),
			zap.String("subnet_id", req.TargetSubnetID),
		)
		l.Info("Received NATS attach resource request")

		err := s.netService.AttachResourceToVPC(ctx, req.TenantID, req.ResourceARN, req.TargetVPCID)
		var privateIP string
		if err == nil {
			if len(req.TargetSubnetID) > 0 {
				var allocErr error
				privateIP, allocErr = s.netService.AttachResourceToSubnet(ctx, req.TenantID, req.ResourceARN, req.TargetVPCID, req.TargetSubnetID)
				if allocErr != nil {
					l.Error("Failed to attach resource to subnet", zap.Error(allocErr))
				}
			} else {
				// No subnet specified, try to get existing assignment's IP
				assignment, aErr := s.netService.ResolveResourceNetwork(ctx, req.ResourceARN)
				if aErr == nil && assignment != nil {
					privateIP = assignment.PrivateIP
				}
			}
		}

		resp := domain.AttachResourceResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
			VPCID:         req.TargetVPCID,
			SubnetID:      req.TargetSubnetID,
			PrivateIP:     privateIP,
			Message:       "Resource successfully attached",
		}
		if err != nil {
			resp.Message = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 6. Detach Resource from Network (Request-Reply)
	_, err = s.nc.Subscribe(SubjectResourceDetach, func(msg *nats.Msg) {
		var req domain.DetachResourceRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(
			zap.String("tenant_id", req.TenantID),
			zap.String("resource_arn", req.ResourceARN),
		)
		l.Info("Received NATS detach resource request")

		err := s.netService.DetachResourceFromNetwork(ctx, req.ResourceARN)
		resp := domain.DetachResourceResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
			Message:       "Resource successfully detached from network",
		}
		if err != nil {
			resp.Message = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 7. Resolve Resource Network (Request-Reply)
	_, err = s.nc.Subscribe(SubjectResourceResolve, func(msg *nats.Msg) {
		var req domain.ResolveResourceRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("resource_arn", req.ResourceARN))
		l.Info("Received NATS resolve resource request")

		assignment, err := s.netService.ResolveResourceNetwork(ctx, req.ResourceARN)
		resp := domain.ResolveResourceResponse{
			CorrelationID: req.CorrelationID,
		}

		if err == nil && assignment != nil {
			resp.VPCID = assignment.VPCID
			resp.SubnetID = assignment.SubnetID
			resp.PrivateIP = assignment.PrivateIP
			resp.PublicIP = assignment.PublicIP
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 8. List Resources in VPC (Request-Reply)
	_, err = s.nc.Subscribe(SubjectVPCResourcesList, func(msg *nats.Msg) {
		var req domain.ListVPCResourcesRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("tenant_id", req.TenantID), zap.String("vpc_id", req.VPCID))
		l.Info("Received NATS list VPC resources request")

		assignments, err := s.netService.ListResourcesInVPC(ctx, req.TenantID, req.VPCID)
		resp := domain.ListVPCResourcesResponse{
			CorrelationID: req.CorrelationID,
			Resources:     []domain.VPCResourceItem{},
		}

		if err == nil {
			for _, a := range assignments {
				resp.Resources = append(resp.Resources, domain.VPCResourceItem{
					ResourceARN: a.ResourceARN,
					SubnetID:    a.SubnetID,
					PrivateIP:   a.PrivateIP,
				})
			}
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 9. Default VPC + Subnet Resolution (Request-Reply)
	_, err = s.nc.Subscribe(SubjectVPCDefaultResolve, func(msg *nats.Msg) {
		var req domain.ResolveDefaultVPCRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("tenant_id", req.TenantID))
		l.Info("Received NATS resolve default network request")

		vpcID, subnetID, bridgeName, err := s.netService.ResolveDefaultNetwork(ctx, req.TenantID)
		resp := domain.ResolveDefaultVPCResponse{
			CorrelationID: req.CorrelationID,
			VPCID:         vpcID,
			SubnetID:      subnetID,
			BridgeName:    bridgeName,
		}

		if err != nil {
			l.Error("Failed to resolve default network", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 10. Register Compute Instance (Request-Reply)
	_, err = s.nc.Subscribe(SubjectComputeRegister, func(msg *nats.Msg) {
		var req domain.RegisterComputeRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("instance_id", req.InstanceID))
		l.Info("Received compute registration request")

		inst := domain.ComputeInstance{
			InstanceID:  req.InstanceID,
			IPAddress:   req.IPAddress,
			ServicePort: req.ServicePort,
			Status:      domain.InstanceStatusStarting,
			Metadata:    req.Metadata,
		}

		err := s.netService.RegisterComputeInstance(ctx, inst)
		resp := map[string]interface{}{
			"correlation_id": req.CorrelationID,
			"success":        err == nil,
		}
		if err != nil {
			resp["error"] = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to compute.register: %w", err)
	}

	// 11. Update Compute Health (Request-Reply)
	_, err = s.nc.Subscribe(SubjectComputeHealth, func(msg *nats.Msg) {
		var req domain.UpdateComputeHealthRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("instance_id", req.InstanceID))
		l.Info("Received compute health update request")

		err := s.netService.UpdateComputeInstanceHealth(ctx, req.InstanceID, req.Status)
		resp := map[string]interface{}{
			"correlation_id": req.CorrelationID,
			"success":        err == nil,
		}
		if err != nil {
			resp["error"] = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to compute.health: %w", err)
	}

	// 12. Deregister Compute Instance (Request-Reply)
	_, err = s.nc.Subscribe(SubjectComputeDeregister, func(msg *nats.Msg) {
		var req domain.DeregisterComputeRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("instance_id", req.InstanceID))
		l.Info("Received compute deregistration request")

		err := s.netService.DeregisterComputeInstance(ctx, req.InstanceID)
		resp := map[string]interface{}{
			"correlation_id": req.CorrelationID,
			"success":        err == nil,
		}
		if err != nil {
			resp["error"] = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to compute.deregister: %w", err)
	}

	// 13. List Active Compute Instances (Request-Reply)
	_, err = s.nc.Subscribe(SubjectComputeList, func(msg *nats.Msg) {
		var req domain.ListComputeInstancesRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Received active compute list request")

		instances, err := s.netService.GetActiveComputeInstances(ctx)
		resp := domain.ListComputeInstancesResponse{
			CorrelationID: req.CorrelationID,
			Instances:     instances,
		}

		if err != nil {
			l.Error("Failed to list active instances", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to compute.list: %w", err)
	}

	// 14. Compute Lifecycle Events (Asynchronous)
	_, err = s.nc.Subscribe(SubjectComputeLifecycle, func(msg *nats.Msg) {
		var event domain.ComputeLifecycleEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, event.CorrelationID)
		l := logger.WithContext(ctx)
		// l.Info("Received compute lifecycle event from NATS", zap.String("instance_id", event.InstanceID), zap.String("type", string(event.EventType)))

		if err := s.netService.HandleComputeLifecycleEvent(ctx, event); err != nil {
			l.Error("Failed to handle compute lifecycle event", zap.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to compute.lifecycle: %w", err)
	}

	// 15. Select Compute Route (Request-Reply)
	_, err = s.nc.Subscribe(SubjectComputeRoute, func(msg *nats.Msg) {
		var req domain.SelectComputeRouteRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Received compute route selection request")

		inst, err := s.netService.SelectComputeRoute(ctx)
		resp := domain.SelectComputeRouteResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
			Instance:      inst,
		}

		if err != nil {
			resp.Error = err.Error()
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 16. Prepare Instance Network — allocates IP from subnet and returns it to EC2
	_, err = s.nc.Subscribe(SubjectInstancePrepare, func(msg *nats.Msg) {
		var req struct {
			CorrelationID string `json:"correlation_id"`
			TenantID      string `json:"tenant_id"`
			InstanceID    string `json:"instance_id"`
			VPCID         string `json:"vpc_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(
			zap.String("tenant_id", req.TenantID),
			zap.String("instance_id", req.InstanceID),
			zap.String("vpc_id", req.VPCID),
		)
		l.Info("Received instance network prepare request")

		// Build the resource ARN for this instance
		resourceARN := fmt.Sprintf("arn:aws:ec2:::%s", req.InstanceID)

		// Step 1: resolve the VPC subnet and bridge
		var vpcID, subnetID, bridgeName string
		if req.VPCID != "" {
			vpcID, subnetID, bridgeName, err = s.netService.ResolveVPCNetwork(ctx, req.TenantID, req.VPCID)
		} else {
			vpcID, subnetID, bridgeName, err = s.netService.ResolveDefaultNetwork(ctx, req.TenantID)
		}

		if err != nil {
			l.Error("Failed to resolve network for instance prepare", zap.Error(err))
			resp, _ := json.Marshal(map[string]interface{}{
				"correlation_id": req.CorrelationID,
				"success":        false,
				"error":          err.Error(),
			})
			s.nc.Publish(msg.Reply, resp)
			return
		}

		// Step 2: attach resource to subnet — this allocates the IP from the CIDR pool
		privateIP, err := s.netService.AttachResourceToSubnet(ctx, req.TenantID, resourceARN, vpcID, subnetID)
		if err != nil {
			l.Error("Failed to allocate IP for instance", zap.Error(err))
			resp, _ := json.Marshal(map[string]interface{}{
				"correlation_id": req.CorrelationID,
				"success":        false,
				"error":          err.Error(),
			})
			s.nc.Publish(msg.Reply, resp)
			return
		}

		// Step 2.1: Automatically allocate and associate an Elastic IP
		// This ensures the VM is reachable from the host network immediately.
		// Only for EC2 instances (i- prefix), not RDS containers which use port-based NAT.
		if strings.HasPrefix(req.InstanceID, "i-") {
			eip, err := s.netService.AutoAssociateEIP(ctx, req.InstanceID, privateIP)
			if err != nil {
				l.Warn("Failed to auto-associate EIP during instance prepare — VM will only have private IP", zap.Error(err))
			} else {
				l.Info("Auto-associated EIP", zap.String("public_ip", eip.PublicIP))
			}
		} else {
			logger.WithContext(ctx).Info("Skipping EIP auto-association for non-EC2 resource",
				zap.String("instance_id", req.InstanceID))
		}

		// Step 3: derive gateway from the bridge — gateway is always x.x.x.1
		// The bridge IP is assigned as x.x.x.1/24 during VPC provisioning
		gateway := service.DeriveGateway(privateIP)

		l.Info("Instance network prepared",
			zap.String("private_ip", privateIP),
			zap.String("gateway", gateway),
			zap.String("bridge", bridgeName),
		)

		resp, _ := json.Marshal(map[string]interface{}{
			"correlation_id": req.CorrelationID,
			"success":        true,
			"private_ip":     privateIP,
			"gateway":        gateway,
			"bridge_name":    bridgeName,
		})
		s.nc.Publish(msg.Reply, resp)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectInstancePrepare, err)
	}

	// 17. Release Instance Network — releases the IP back to the subnet pool
	_, err = s.nc.Subscribe(SubjectInstanceRelease, func(msg *nats.Msg) {
		var req struct {
			CorrelationID string `json:"correlation_id"`
			TenantID      string `json:"tenant_id"`
			InstanceID    string `json:"instance_id"`
			VPCID         string `json:"vpc_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(
			zap.String("tenant_id", req.TenantID),
			zap.String("instance_id", req.InstanceID),
		)
		l.Info("Received instance network release request")

		resourceARN := fmt.Sprintf("arn:aws:ec2:::%s", req.InstanceID)

		err := s.netService.DetachResourceFromNetwork(ctx, resourceARN)

		status := "success"
		errMsg := ""
		if err != nil {
			l.Error("Failed to release instance network", zap.Error(err))
			status = "error"
			errMsg = err.Error()
		} else {
			l.Info("Instance network released successfully")
		}

		resp, _ := json.Marshal(map[string]interface{}{
			"correlation_id": req.CorrelationID,
			"status":         status,
			"error":          errMsg,
		})
		s.nc.Publish(msg.Reply, resp)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectInstanceRelease, err)
	}

	if err != nil {
		return fmt.Errorf("failed to subscribe to compute.route: %w", err)
	}

	// 18. Allocate EIP (Request-Reply)
	_, err = s.nc.Subscribe(SubjectEIPAllocate, func(msg *nats.Msg) {
		var req domain.AllocateEIPRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Received EIP allocation request")

		eip, err := s.netService.AllocateEIP(ctx)
		resp := domain.AllocateEIPResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
		}

		if err == nil {
			resp.EIPID = eip.ID
			resp.PublicIP = eip.PublicIP
			l.Info("Allocated EIP", zap.String("public_ip", eip.PublicIP))
		} else {
			resp.Error = err.Error()
			l.Error("Failed to allocate EIP", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 19. Associate EIP (Request-Reply)
	_, err = s.nc.Subscribe(SubjectEIPAssociate, func(msg *nats.Msg) {
		var req domain.AssociateEIPRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("eip_id", req.EIPID), zap.String("instance_id", req.InstanceID))
		l.Info("Received EIP association request")

		// Step 1: Resolve instance private IP
		resourceARN := fmt.Sprintf("arn:aws:ec2:::%s", req.InstanceID)
		assignment, err := s.netService.ResolveResourceNetwork(ctx, resourceARN)
		if err != nil {
			l.Error("Failed to resolve instance network for EIP association", zap.Error(err))
			resp, _ := json.Marshal(domain.AssociateEIPResponse{
				CorrelationID: req.CorrelationID,
				Success:       false,
				Error:         fmt.Sprintf("failed to resolve instance network: %v", err),
			})
			s.nc.Publish(msg.Reply, resp)
			return
		}

		// Step 2: Associate
		err = s.netService.AssociateEIP(ctx, req.EIPID, req.InstanceID, assignment.PrivateIP)
		resp := domain.AssociateEIPResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
		}
		if err != nil {
			resp.Error = err.Error()
			l.Error("Failed to associate EIP", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 20. Disassociate EIP (Request-Reply)
	_, err = s.nc.Subscribe(SubjectEIPDisassociate, func(msg *nats.Msg) {
		var req domain.DisassociateEIPRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("eip_id", req.EIPID))
		l.Info("Received EIP disassociation request")

		err := s.netService.DisassociateEIP(ctx, req.EIPID)
		resp := domain.DisassociateEIPResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
		}
		if err != nil {
			resp.Error = err.Error()
			l.Error("Failed to disassociate EIP", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 20b. Reconcile VPCs (Request-Reply)
	_, err = s.nc.Subscribe(SubjectVPCReconcile, func(msg *nats.Msg) {
		var req domain.ReconcileVPCsRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx)
		l.Info("Received VPC reconciliation request")

		err := s.netService.ReconcileVPCs(ctx)
		resp := domain.ReconcileVPCsResponse{
			CorrelationID: req.CorrelationID,
			Success:       err == nil,
		}

		if err == nil {
			resp.Message = "VPC reconciliation completed successfully"
		} else {
			resp.Error = err.Error()
			l.Error("Failed to reconcile VPCs", zap.Error(err))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})

	// 21. Expose RDS Container (Request-Reply)
	_, err = s.nc.Subscribe(SubjectRDSExpose, func(msg *nats.Msg) {
		var req struct {
			CorrelationID string `json:"correlation_id"`
			TenantID      string `json:"tenant_id"`
			ResourceID    string `json:"resource_id"`
			PrivateIP     string `json:"private_ip"`
			PrivatePort   int    `json:"private_port"`
			PublicIP      string `json:"public_ip"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(
			zap.String("tenant_id", req.TenantID),
			zap.String("resource_id", req.ResourceID),
			zap.String("private_ip", req.PrivateIP),
			zap.Int("private_port", req.PrivatePort),
			zap.String("public_ip", req.PublicIP),
		)
		l.Info("[RDS-EXPOSE] Received expose request")

		publicPort, err := s.netService.ExposeRDSContainer(ctx, req.TenantID, req.ResourceID, req.PrivateIP, req.PublicIP, req.PrivatePort)

		resp := struct {
			Success    bool   `json:"success"`
			PublicPort int    `json:"public_port"`
			Error      string `json:"error"`
		}{
			Success:    err == nil,
			PublicPort: publicPort,
		}
		if err != nil {
			resp.Error = err.Error()
			l.Error("[RDS-EXPOSE] Failed to expose RDS container", zap.Error(err))
		} else {
			l.Info("[RDS-EXPOSE] Successfully exposed RDS container", zap.Int("public_port", publicPort))
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectRDSExpose, err)
	}

	// 22. Unexpose RDS Container (Request-Reply)
	_, err = s.nc.Subscribe(SubjectRDSUnexpose, func(msg *nats.Msg) {
		var req struct {
			CorrelationID string `json:"correlation_id"`
			ResourceID    string `json:"resource_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return
		}

		ctx := context.WithValue(context.Background(), logger.CorrelationIDKey, req.CorrelationID)
		l := logger.WithContext(ctx).With(zap.String("resource_id", req.ResourceID))
		l.Info("[RDS-EXPOSE] Received unexpose request")

		err := s.netService.UnexposeRDSContainer(ctx, req.ResourceID)

		resp := struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}{
			Success: err == nil,
		}
		if err != nil {
			resp.Error = err.Error()
			l.Error("[RDS-EXPOSE] Failed to unexpose RDS container", zap.Error(err))
		} else {
			l.Info("[RDS-EXPOSE] Successfully unexposed RDS container")
		}

		data, _ := json.Marshal(resp)
		s.nc.Publish(msg.Reply, data)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", SubjectRDSUnexpose, err)
	}

	return nil
}
