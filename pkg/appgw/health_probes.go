// -------------------------------------------------------------------------------------------
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.
// --------------------------------------------------------------------------------------------

package appgw

import (
	"fmt"
	"sort"

	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/annotations"
	"github.com/Azure/application-gateway-kubernetes-ingress/pkg/sorter"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-12-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *appGwConfigBuilder) newProbesMap(ingressList []*v1beta1.Ingress) (map[string]network.ApplicationGatewayProbe, map[backendIdentifier](*network.ApplicationGatewayProbe)) {
	healthProbeCollection := make(map[string]network.ApplicationGatewayProbe)
	backendIDs := newBackendIds(ingressList)
	probesMap := make(map[backendIdentifier]*network.ApplicationGatewayProbe)
	defaultProbe := defaultProbe()

	glog.Info("[health-probes] Adding default probe:", *defaultProbe.Name)
	healthProbeCollection[*defaultProbe.Name] = defaultProbe

	for backendID := range backendIDs {
		probe := c.generateHealthProbe(backendID)

		if probe != nil {
			glog.Infof("[health-probes] Created probe %s for backend: '%s'", *probe.Name, backendID.Name)
			probesMap[backendID] = probe
			healthProbeCollection[*probe.Name] = *probe
		} else {
			glog.Infof("[health-probes] No k8s probe for backend: '%s'; Adding default probe: '%s'", backendID.Name, *defaultProbe.Name)
			probesMap[backendID] = &defaultProbe
		}
	}
	return healthProbeCollection, probesMap
}

func (c *appGwConfigBuilder) HealthProbesCollection(ingressList []*v1beta1.Ingress) error {
	healthProbeCollection, _ := c.newProbesMap(ingressList)

	glog.Infof("[health-probes] Will create %d App Gateway probes.", len(healthProbeCollection))

	probes := make([]network.ApplicationGatewayProbe, 0, len(healthProbeCollection))
	for _, probe := range healthProbeCollection {
		probes = append(probes, probe)
	}

	sort.Sort(sorter.ByHealthProbeName(probes))

	c.appGwConfig.Probes = &probes
	return nil
}

func (c *appGwConfigBuilder) generateHealthProbe(backendID backendIdentifier) *network.ApplicationGatewayProbe {
	service := c.k8sContext.GetService(backendID.serviceKey())
	if service == nil {
		return nil
	}

	probe := defaultProbe()
	probe.Name = to.StringPtr(generateProbeName(backendID.Path.Backend.ServiceName, backendID.Path.Backend.ServicePort.String(), backendID.Ingress.Name))
	if backendID.Rule != nil && len(backendID.Rule.Host) != 0 {
		probe.Host = to.StringPtr(backendID.Rule.Host)
	}

	pathPrefix, err := annotations.BackendPathPrefix(backendID.Ingress)
	if err == nil {
		probe.Path = to.StringPtr(pathPrefix)
	} else if backendID.Path != nil && len(backendID.Path.Path) != 0 {
		probe.Path = to.StringPtr(backendID.Path.Path)
	}

	k8sProbeForServiceContainer := c.getProbeForServiceContainer(service, backendID)
	if k8sProbeForServiceContainer != nil {
		if len(k8sProbeForServiceContainer.Handler.HTTPGet.Host) != 0 {
			probe.Host = to.StringPtr(k8sProbeForServiceContainer.Handler.HTTPGet.Host)
		}
		if len(k8sProbeForServiceContainer.Handler.HTTPGet.Path) != 0 {
			probe.Path = to.StringPtr(k8sProbeForServiceContainer.Handler.HTTPGet.Path)
		}
		if k8sProbeForServiceContainer.Handler.HTTPGet.Scheme == v1.URISchemeHTTPS {
			probe.Protocol = network.HTTPS
		}
		if k8sProbeForServiceContainer.PeriodSeconds != 0 {
			probe.Interval = to.Int32Ptr(k8sProbeForServiceContainer.PeriodSeconds)
		}
		if k8sProbeForServiceContainer.TimeoutSeconds != 0 {
			probe.Timeout = to.Int32Ptr(k8sProbeForServiceContainer.TimeoutSeconds)
		}
		if k8sProbeForServiceContainer.FailureThreshold != 0 {
			probe.UnhealthyThreshold = to.Int32Ptr(k8sProbeForServiceContainer.FailureThreshold)
		}
	}

	return &probe
}

func (c *appGwConfigBuilder) getProbeForServiceContainer(service *v1.Service, backendID backendIdentifier) *v1.Probe {
	allPorts := make(map[int32]interface{})
	for _, sp := range service.Spec.Ports {
		if sp.Protocol != v1.ProtocolTCP {
			continue
		}

		if fmt.Sprint(sp.Port) == backendID.Backend.ServicePort.String() ||
			sp.Name == backendID.Backend.ServicePort.String() ||
			sp.TargetPort.String() == backendID.Backend.ServicePort.String() {

			// Matched a service port in the service
			if sp.TargetPort.String() == "" {
				allPorts[sp.Port] = nil
			} else if sp.TargetPort.Type == intstr.Int {
				// port is defined as port number
				allPorts[sp.TargetPort.IntVal] = nil
			} else {
				for targetPort := range c.resolvePortName(sp.TargetPort.StrVal, &backendID) {
					allPorts[targetPort] = nil
				}
			}
		}
	}

	podList := c.k8sContext.GetPodsByServiceSelector(service.Spec.Selector)
	for _, pod := range podList {
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				if _, ok := allPorts[port.ContainerPort]; !ok {
					continue
				}

				var probe *v1.Probe
				if container.ReadinessProbe != nil && container.ReadinessProbe.Handler.HTTPGet != nil {
					probe = container.ReadinessProbe
				} else if container.LivenessProbe != nil && container.LivenessProbe.Handler.HTTPGet != nil {
					probe = container.LivenessProbe
				}

				return probe
			}
		}
	}

	return nil
}
