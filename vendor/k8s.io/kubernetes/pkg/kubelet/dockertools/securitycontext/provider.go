/*
Copyright 2014 The Kubernetes Authors.

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

package securitycontext

import (
	"fmt"
	"strconv"

	"k8s.io/kubernetes/pkg/api/v1"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/leaky"
	"k8s.io/kubernetes/pkg/securitycontext"

	dockercontainer "github.com/docker/engine-api/types/container"
)

// NewSimpleSecurityContextProvider creates a new SimpleSecurityContextProvider.
func NewSimpleSecurityContextProvider() SecurityContextProvider {
	return SimpleSecurityContextProvider{}
}

// SimpleSecurityContextProvider is the default implementation of a SecurityContextProvider.
type SimpleSecurityContextProvider struct{}

// ModifyContainerConfig is called before the Docker createContainer call.
// The security context provider can make changes to the Config with which
// the container is created.
func (p SimpleSecurityContextProvider) ModifyContainerConfig(pod *v1.Pod, container *v1.Container, config *dockercontainer.Config) {
	effectiveSC := securitycontext.DetermineEffectiveSecurityContext(pod, container)
	if effectiveSC == nil {
		return
	}
	if effectiveSC.RunAsUser != nil {
		config.User = strconv.Itoa(int(*effectiveSC.RunAsUser))
	}
}

// ModifyHostConfig is called before the Docker runContainer call. The
// security context provider can make changes to the HostConfig, affecting
// security options, whether the container is privileged, volume binds, etc.
func (p SimpleSecurityContextProvider) ModifyHostConfig(pod *v1.Pod, container *v1.Container, hostConfig *dockercontainer.HostConfig, supplementalGids []int64) {
	// Apply supplemental groups
	if container.Name != leaky.PodInfraContainerName {
		// TODO: We skip application of supplemental groups to the
		// infra container to work around a runc issue which
		// requires containers to have the '/etc/group'. For
		// more information see:
		// https://github.com/opencontainers/runc/pull/313
		// This can be removed once the fix makes it into the
		// required version of docker.
		if pod.Spec.SecurityContext != nil {
			for _, group := range pod.Spec.SecurityContext.SupplementalGroups {
				hostConfig.GroupAdd = append(hostConfig.GroupAdd, strconv.Itoa(int(group)))
			}
			if pod.Spec.SecurityContext.FSGroup != nil {
				hostConfig.GroupAdd = append(hostConfig.GroupAdd, strconv.Itoa(int(*pod.Spec.SecurityContext.FSGroup)))
			}
		}

		for _, group := range supplementalGids {
			hostConfig.GroupAdd = append(hostConfig.GroupAdd, strconv.Itoa(int(group)))
		}
	}

	// Apply effective security context for container
	effectiveSC := securitycontext.DetermineEffectiveSecurityContext(pod, container)
	if effectiveSC == nil {
		return
	}

	if effectiveSC.Privileged != nil {
		hostConfig.Privileged = *effectiveSC.Privileged
	}

	if effectiveSC.Capabilities != nil {
		add, drop := kubecontainer.MakeCapabilities(effectiveSC.Capabilities.Add, effectiveSC.Capabilities.Drop)
		hostConfig.CapAdd = add
		hostConfig.CapDrop = drop
	}

	if effectiveSC.SELinuxOptions != nil {
		hostConfig.SecurityOpt = ModifySecurityOptions(hostConfig.SecurityOpt, effectiveSC.SELinuxOptions)
	}
}

// ModifySecurityOptions adds SELinux options to config.
func ModifySecurityOptions(config []string, selinuxOpts *v1.SELinuxOptions) []string {
	config = modifySecurityOption(config, DockerLabelUser, selinuxOpts.User)
	config = modifySecurityOption(config, DockerLabelRole, selinuxOpts.Role)
	config = modifySecurityOption(config, DockerLabelType, selinuxOpts.Type)
	config = modifySecurityOption(config, DockerLabelLevel, selinuxOpts.Level)

	return config
}

// modifySecurityOption adds the security option of name to the config array with value in the form
// of name:value
func modifySecurityOption(config []string, name, value string) []string {
	if len(value) > 0 {
		config = append(config, fmt.Sprintf("%s:%s", name, value))
	}
	return config
}