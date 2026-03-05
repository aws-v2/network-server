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

	SubjectVPCDefaultGet     = "dev.network.v1.vpc.default.get"
	SubjectVPCValidate       = "dev.network.v1.vpc.validate"
	SubjectVPCDefaultResolve = "dev.network.v1.vpc.default.resolve"

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

    // Step 1: resolve the default VPC subnet and bridge
    vpcID, subnetID, bridgeName, err := s.netService.ResolveDefaultNetwork(ctx, req.TenantID)
    if err != nil {
        l.Error("Failed to resolve default network for instance prepare", zap.Error(err))
        resp, _ := json.Marshal(map[string]interface{}{
            "correlation_id": req.CorrelationID,
            "success":        false,
            "error":          err.Error(),
        })
        s.nc.Publish(msg.Reply, resp)
        return
    }

    // Use the requested VPC if provided, otherwise use default
    if req.VPCID != "" {
        vpcID = req.VPCID
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

    // Step 3: derive gateway from the bridge — gateway is always x.x.x.1
    // The bridge IP is assigned as x.x.x.1/24 during VPC provisioning
    gateway := deriveGateway(privateIP)

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

	return nil
}




// deriveGateway returns the gateway IP for a given private IP.
// By convention, VPC subnets are provisioned with the gateway at x.x.x.1,
// so 10.0.1.5 → gateway is 10.0.1.1
func deriveGateway(privateIP string) string {
    parts := strings.Split(privateIP, ".")
    if len(parts) != 4 {
        return ""
    }
    return fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
}