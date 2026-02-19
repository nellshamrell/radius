/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aspire

import (
	"fmt"
	"strings"
)

// classify determines the ResourceKind for a manifest resource based on its type,
// image, and any user overrides.
func classify(name string, resource ManifestResource, overrides map[string]ResourceKind) ResourceKind {
	// Check for user overrides first.
	if overrides != nil {
		if kind, ok := overrides[name]; ok {
			return kind
		}
	}

	switch {
	case strings.HasPrefix(resource.Type, "container.v"):
		// Check for backing service detection.
		if resource.Image != "" {
			if kind := detectBackingService(resource.Image); kind != KindUnsupported {
				return kind
			}
		}

		return KindContainer

	case strings.HasPrefix(resource.Type, "project.v"):
		return KindContainer

	case resource.Type == "value.v0":
		return KindValueResource

	case resource.Type == "parameter.v0":
		return KindParameter

	default:
		return KindUnsupported
	}
}

// mapContainer converts a ManifestResource into a RadiusResource with ContainerSpec.
func mapContainer(name string, resource ManifestResource, bicepID string, imageMappings map[string]string) (*RadiusResource, error) {
	image := resource.Image

	// For project resources, look up image in mappings.
	if strings.HasPrefix(resource.Type, "project.v") {
		mappedImage, ok := imageMappings[name]
		if !ok {
			return nil, &missingImageMappingError{resourceName: name}
		}

		image = mappedImage
	}

	if image == "" {
		return nil, fmt.Errorf("resource %q has no image", name)
	}

	container := &ContainerSpec{
		Image: image,
		Env:   make(map[string]EnvVarSpec),
		Ports: make(map[string]PortSpec),
	}

	// Map entrypoint to command.
	if resource.Entrypoint != "" {
		container.Command = []string{resource.Entrypoint}
	}

	// Map args.
	if len(resource.Args) > 0 {
		container.Args = resource.Args
	}

	// Map env vars (initially as literal values — expressions are resolved later).
	for key, value := range resource.Env {
		container.Env[key] = EnvVarSpec{Value: value}
	}

	// Map bindings to ports.
	for bindingName, binding := range resource.Bindings {
		port := binding.TargetPort
		if port == 0 {
			port = binding.Port
		}

		if port > 0 {
			container.Ports[bindingName] = PortSpec{
				ContainerPort: port,
				Protocol:      strings.ToUpper(binding.Protocol),
				Scheme:        binding.Scheme,
			}
		}
	}

	// Map volumes.
	if len(resource.Volumes) > 0 {
		container.Volumes = make(map[string]VolumeSpec, len(resource.Volumes))
		for _, vol := range resource.Volumes {
			container.Volumes[vol.Name] = VolumeSpec{
				Kind:      "ephemeral",
				MountPath: vol.Target,
				ReadOnly:  vol.ReadOnly,
			}
		}
	}

	// Map bind mounts as volumes.
	for _, bm := range resource.BindMounts {
		if container.Volumes == nil {
			container.Volumes = make(map[string]VolumeSpec)
		}

		// Generate a volume name from the target path.
		volName := sanitize(bm.Target)
		container.Volumes[volName] = VolumeSpec{
			Kind:      "ephemeral",
			MountPath: bm.Target,
			ReadOnly:  bm.ReadOnly,
		}
	}

	return &RadiusResource{
		BicepIdentifier: bicepID,
		RuntimeName:     name,
		RadiusType:      string(KindContainer),
		APIVersion:      apiVersion,
		Kind:            KindContainer,
		Container:       container,
	}, nil
}

// mapPortableResource creates a RadiusResource for a portable resource (backing service).
func mapPortableResource(name string, kind ResourceKind, bicepID string) *RadiusResource {
	return &RadiusResource{
		BicepIdentifier: bicepID,
		RuntimeName:     name,
		RadiusType:      string(kind),
		APIVersion:      apiVersion,
		Kind:            kind,
		PortableResource: &PortableResourceSpec{
			RecipeName: "default",
		},
	}
}

// synthesizeApplication creates the Applications.Core/applications resource.
func synthesizeApplication(appName, environmentName string) *RadiusResource {
	return &RadiusResource{
		BicepIdentifier: "app",
		RuntimeName:     appName,
		RadiusType:      string(KindApplication),
		APIVersion:      apiVersion,
		Kind:            KindApplication,
		Application: &ApplicationSpec{
			EnvironmentRef: environmentName,
		},
	}
}

// synthesizeGateway creates a gateway resource from container bindings marked as external.
func synthesizeGateway(ctx *translationContext) *RadiusResource {
	var routes []GatewayRouteSpec

	for name, resource := range ctx.manifest.Resources {
		if resource.Bindings == nil {
			continue
		}

		for _, binding := range resource.Bindings {
			if !binding.External {
				continue
			}

			url := buildBindingURL(name, binding)
			routes = append(routes, GatewayRouteSpec{
				Path:        "/",
				Destination: url,
			})
		}
	}

	if len(routes) == 0 {
		return nil
	}

	return &RadiusResource{
		BicepIdentifier: "gateway",
		RuntimeName:     "gateway",
		RadiusType:      string(KindGateway),
		APIVersion:      apiVersion,
		Kind:            KindGateway,
		Gateway: &GatewaySpec{
			Routes: routes,
		},
	}
}
