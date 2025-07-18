// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/imdario/mergo"
	"github.com/sagikazarmark/docker-ref/reference"
	"github.com/spf13/cast"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	extv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	log = ctrl.Log.WithName("controller_vault")

	// DefaultBankVaultsImage defines the image used when VaultSpec.BankVaultsImage is empty.
	DefaultBankVaultsImage = "ghcr.io/bank-vaults/bank-vaults:latest"

	// HAStorageTypes is the set of storage backends supporting High Availability
	HAStorageTypes = map[string]bool{
		"consul":     true,
		"dynamodb":   true,
		"etcd":       true,
		"gcs":        true,
		"mysql":      true,
		"postgresql": true,
		"raft":       true,
		"oci":        true,
		"spanner":    true,
		"zookeeper":  true,
	}
)

// VaultSpec defines the desired state of Vault
type VaultSpec struct {
	// Size defines the number of Vault instances in the cluster (>= 1 means HA)
	// default: 1
	Size int32 `json:"size,omitempty"`

	// Image specifies the Vault image to use for the Vault instances
	// default: hashicorp/vault:latest
	Image string `json:"image,omitempty"`

	// BankVaultsImage specifies the Bank Vaults image to use for Vault unsealing and configuration
	// default: ghcr.io/bank-vaults/bank-vaults:latest
	BankVaultsImage string `json:"bankVaultsImage,omitempty"`

	// BankVaultsVolumeMounts define some extra Kubernetes Volume mounts for the Bank Vaults Sidecar container.
	// default:
	BankVaultsVolumeMounts []v1.VolumeMount `json:"bankVaultsVolumeMounts,omitempty"`

	// StatsDDisabled specifies if StatsD based metrics should be disabled
	// default: false
	StatsDDisabled bool `json:"statsdDisabled,omitempty"`

	// StatsDImage specifies the StatsD image to use for Vault metrics exportation
	// default: prom/statsd-exporter:latest
	StatsDImage string `json:"statsdImage,omitempty"`

	// StatsdConfig specifies the StatsD mapping configuration
	// default:
	StatsdConfig string `json:"statsdConfig,omitempty"`

	// FluentDEnabled specifies if FluentD based log exportation should be enabled
	// default: false
	FluentDEnabled bool `json:"fluentdEnabled,omitempty"`

	// FluentDImage specifies the FluentD image to use for Vault log exportation
	// default: fluent/fluentd:edge
	FluentDImage string `json:"fluentdImage,omitempty"`

	// FluentDConfLocation is the location of the fluent.conf file
	// default: "/fluentd/etc"
	FluentDConfLocation string `json:"fluentdConfLocation,omitempty"`

	// FluentDConfFile specifies the FluentD configuration file name to use for Vault log exportation
	// default:
	FluentDConfFile string `json:"fluentdConfFile,omitempty"`

	// FluentDConfig specifies the FluentD configuration to use for Vault log exportation
	// default:
	FluentDConfig string `json:"fluentdConfig,omitempty"`

	// WatchedSecretsLabels specifies a set of Kubernetes label selectors which select Secrets to watch.
	// If these Secrets change the Vault cluster gets restarted. For example a Secret that Cert-Manager is
	// managing a public Certificate for Vault using let's Encrypt.
	// default:
	WatchedSecretsLabels []map[string]string `json:"watchedSecretsLabels,omitempty"`

	// WatchedSecretsAnnotations specifies a set of Kubernetes annotations selectors which select Secrets to watch.
	// If these Secrets change the Vault cluster gets restarted. For example a Secret that Cert-Manager is
	// managing a public Certificate for Vault using let's Encrypt.
	// default:
	WatchedSecretsAnnotations []map[string]string `json:"watchedSecretsAnnotations,omitempty"`

	// Annotations define a set of common Kubernetes annotations that will be added to all operator managed resources.
	// default:
	Annotations map[string]string `json:"annotations,omitempty"`

	// VaultAnnotations define a set of Kubernetes annotations that will be added to all Vault Pods.
	// default:
	VaultAnnotations map[string]string `json:"vaultAnnotations,omitempty"`

	// VaultLabels define a set of Kubernetes labels that will be added to all Vault Pods.
	// default:
	VaultLabels map[string]string `json:"vaultLabels,omitempty"`

	// VaultPodSpec is a Kubernetes Pod specification snippet (`spec:` block) that will be merged into the operator generated
	// Vault Pod specification.
	// default:
	VaultPodSpec *EmbeddedPodSpec `json:"vaultPodSpec,omitempty"`

	// VaultContainerSpec is a Kubernetes Container specification snippet that will be merged into the operator generated
	// Vault Container specification.
	// default:
	VaultContainerSpec v1.Container `json:"vaultContainerSpec,omitempty"`

	// VaultConfigurerAnnotations define a set of Kubernetes annotations that will be added to the Vault Configurer Pod.
	// default:
	VaultConfigurerAnnotations map[string]string `json:"vaultConfigurerAnnotations,omitempty"`

	// VaultConfigurerLabels define a set of Kubernetes labels that will be added to all Vault Configurer Pod.
	// default:
	VaultConfigurerLabels map[string]string `json:"vaultConfigurerLabels,omitempty"`

	// VaultConfigurerPodSpec is a Kubernetes Pod specification snippet (`spec:` block) that will be merged into
	// the operator generated Vault Configurer Pod specification.
	// default:
	VaultConfigurerPodSpec *EmbeddedPodSpec `json:"vaultConfigurerPodSpec,omitempty"`

	// ConfigPath describes where to store configuration file
	// default: "/vault/config"
	ConfigPath string `json:"configPath,omitempty"`

	// Config is the Vault Server configuration. See https://www.vaultproject.io/docs/configuration/ for more details.
	// default:
	Config extv1beta1.JSON `json:"config"`

	// ExternalConfig is higher level configuration block which instructs the Bank Vaults Configurer to configure Vault
	// through its API, thus allows setting up:
	// - Secret Engines
	// - Auth Methods
	// - Audit Devices
	// - Plugin Backends
	// - Policies
	// - Startup Secrets (Bank Vaults feature)
	// A documented example: https://github.com/bank-vaults/vault-operator/blob/main/vault-config.yml
	// default:
	ExternalConfig extv1beta1.JSON `json:"externalConfig,omitempty"`

	// UnsealConfig defines where the Vault cluster's unseal keys and root token should be stored after initialization.
	// See the type's documentation for more details. Only one method may be specified.
	// default: Kubernetes Secret based unsealing
	UnsealConfig UnsealConfig `json:"unsealConfig,omitempty"`

	// CredentialsConfig defines a external Secret for Vault and how it should be mounted to the Vault Pod
	// for example accessing Cloud resources.
	// default:
	CredentialsConfig CredentialsConfig `json:"credentialsConfig,omitempty"`

	// EnvsConfig is a list of Kubernetes environment variable definitions that will be passed to all Bank-Vaults pods.
	// default:
	EnvsConfig []v1.EnvVar `json:"envsConfig,omitempty"`

	// SecurityContext is a Kubernetes PodSecurityContext that will be applied to all Pods created by the operator.
	// default:
	SecurityContext v1.PodSecurityContext `json:"securityContext,omitempty"`

	// ServiceType is a Kubernetes Service type of the Vault Service.
	// default: ClusterIP
	ServiceType string `json:"serviceType,omitempty"`

	// LoadBalancerIP is an optional setting for allocating a specific address for the entry service object
	// of type LoadBalancer
	// default: ""
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`

	// serviceRegistrationEnabled enables the injection of the service_registration Vault stanza.
	// This requires elaborated RBAC privileges for updating Pod labels for the Vault Pod.
	// default: false
	ServiceRegistrationEnabled bool `json:"serviceRegistrationEnabled,omitempty"`

	// RaftLeaderAddress defines the leader address of the raft cluster in multi-cluster deployments.
	// (In single cluster (namespace) deployments it is automatically detected).
	// "self" is a special value which means that this instance should be the bootstrap leader instance.
	// default: ""
	RaftLeaderAddress string `json:"raftLeaderAddress,omitempty"`

	// RaftLeaderApiSchemeOverride will override the value provided from TLS defined values in order
	// to handle TLS being terminated by an external reverse proxy, load balancer, etc.
	// default: ""
	RaftLeaderApiSchemeOverride string `json:"raftLeaderApiSchemeOverride,omitempty"`

	// ServicePorts is an extra map of ports that should be exposed by the Vault Service.
	// default:
	ServicePorts map[string]int32 `json:"servicePorts,omitempty"`

	// Affinity is a group of affinity scheduling rules applied to all Vault Pods.
	// default:
	Affinity *v1.Affinity `json:"affinity,omitempty"`

	// PodAntiAffinity is the TopologyKey in the Vault Pod's PodAntiAffinity.
	// No PodAntiAffinity is used if empty.
	// Deprecated. Use Affinity.
	// default:
	PodAntiAffinity string `json:"podAntiAffinity,omitempty"`

	// NodeAffinity is Kubernetees NodeAffinity definition that should be applied to all Vault Pods.
	// Deprecated. Use Affinity.
	// default:
	NodeAffinity v1.NodeAffinity `json:"nodeAffinity,omitempty"`

	// NodeSelector is Kubernetees NodeSelector definition that should be applied to all Vault Pods.
	// default:
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations is Kubernetes Tolerations definition that should be applied to all Vault Pods.
	// default:
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// ServiceAccount is Kubernetes ServiceAccount in which the Vault Pods should be running in.
	// default: default
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Volumes define some extra Kubernetes Volumes for the Vault Pods.
	// default:
	Volumes []v1.Volume `json:"volumes,omitempty"`

	// VolumeMounts define some extra Kubernetes Volume mounts for the Vault Pods.
	// default:
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`

	// VolumeClaimTemplates define some extra Kubernetes PersistentVolumeClaim templates for the Vault Statefulset.
	// default:
	VolumeClaimTemplates []EmbeddedPersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`

	// DEPRECATED: Use SecretInitsConfig instead
	// VaultEnvsConfig is a list of Kubernetes environment variable definitions that will be passed to the Vault container.
	// default:
	VaultEnvsConfig []v1.EnvVar `json:"vaultEnvsConfig,omitempty"`

	// SecretInitsConfig is a list of Kubernetes environment variable definitions that will be passed to the Vault container.
	// default:
	SecretInitsConfig []v1.EnvVar `json:"secretInitsConfig,omitempty"`

	// SidecarEnvsConfig is a list of Kubernetes environment variable definitions that will be passed to Vault sidecar containers.
	// default:
	SidecarEnvsConfig []v1.EnvVar `json:"sidecarEnvsConfig,omitempty"`

	// Resources defines the resource limits for all the resources created by the operator.
	// See the type for more details.
	// default:
	Resources *Resources `json:"resources,omitempty"`

	// Ingress, if it is specified the operator will create an Ingress resource for the Vault Service and
	// will annotate it with the correct Ingress annotations specific to the TLS settings in the configuration.
	// See the type for more details.
	// default:
	Ingress *Ingress `json:"ingress,omitempty"`

	// ServiceMonitorEnabled enables the creation of Prometheus Operator specific ServiceMonitor for Vault.
	// default: false
	ServiceMonitorEnabled bool `json:"serviceMonitorEnabled,omitempty"`

	// ExistingTLSSecretName is name of the secret that contains a TLS server certificate and key and the corresponding CA certificate.
	// Required secret format kubernetes.io/tls type secret keys + ca.crt key
	// If it is set, generating certificate will be disabled
	// default: ""
	ExistingTLSSecretName string `json:"existingTlsSecretName,omitempty"`

	// TLSExpiryThreshold is the Vault TLS certificate expiration threshold in Go's Duration format.
	// default: 168h
	TLSExpiryThreshold string `json:"tlsExpiryThreshold,omitempty"`

	// TLSAdditionalHosts is a list of additional hostnames or IP addresses to add to the SAN on the automatically generated TLS certificate.
	// default:
	TLSAdditionalHosts []string `json:"tlsAdditionalHosts,omitempty"`

	// CANamespaces define a list of namespaces where the generated CA certificate for Vault should be distributed,
	// use ["*"] for all namespaces.
	// default:
	CANamespaces []string `json:"caNamespaces,omitempty"`

	// IstioEnabled describes if the cluster has a Istio running and enabled.
	// default: false
	IstioEnabled bool `json:"istioEnabled,omitempty"`

	// VeleroEnabled describes if the cluster has a Velero running and enabled.
	// default: false
	VeleroEnabled bool `json:"veleroEnabled,omitempty"`

	// VeleroFsfreezeImage specifies the Velero Fsrfeeze image to use in Velero backup hooks
	// default: velero/fsfreeze-pause:latest
	VeleroFsfreezeImage string `json:"veleroFsfreezeImage,omitempty"`

	// VaultContainers add extra containers
	VaultContainers []v1.Container `json:"vaultContainers,omitempty"`

	// VaultInitContainers add extra initContainers
	VaultInitContainers []v1.Container `json:"vaultInitContainers,omitempty"`
}

// HasHAStorage detects if Vault is configured to use a storage backend which supports High Availability or if it has
// ha_storage stanza, then doesn't check for ha_enabled flag
func (spec *VaultSpec) HasHAStorage() bool {
	storageType := spec.GetStorageType()
	if _, ok := HAStorageTypes[storageType]; ok {
		if spec.HasStorageHAEnabled() {
			return true
		}
	}
	if spec.hasHAStorageStanza() {
		return true
	}
	return false
}

func (spec *VaultSpec) hasHAStorageStanza() bool {
	return len(spec.getHAStorage()) != 0
}

// GetStorage returns Vault's storage stanza
func (spec *VaultSpec) GetStorage() map[string]interface{} {
	storage := spec.getStorage()
	return cast.ToStringMap(storage[spec.GetStorageType()])
}

func (spec *VaultSpec) getStorage() map[string]interface{} {
	config := spec.GetVaultConfig()
	return cast.ToStringMap(config["storage"])
}

// GetHAStorage returns Vault's ha_storage stanza
func (spec *VaultSpec) GetHAStorage() map[string]interface{} {
	haStorage := spec.getHAStorage()
	return cast.ToStringMap(haStorage[spec.GetHAStorageType()])
}

func (spec *VaultSpec) getHAStorage() map[string]interface{} {
	config := spec.GetVaultConfig()
	return cast.ToStringMap(config["ha_storage"])
}

func (spec *VaultSpec) GetVaultConfig() map[string]interface{} {
	var config map[string]interface{}
	// This config JSON is already validated,
	// so we can skip wiring through the error everywhere.
	_ = json.Unmarshal(spec.Config.Raw, &config)
	return config
}

// GetStorageType returns the type of Vault's storage stanza
func (spec *VaultSpec) GetStorageType() string {
	storage := spec.getStorage()
	keys := reflect.ValueOf(storage).MapKeys()
	if len(keys) == 0 {
		return ""
	}
	return keys[0].String()
}

// GetHAStorageType returns the type of Vault's ha_storage stanza
func (spec *VaultSpec) GetHAStorageType() string {
	haStorage := spec.getHAStorage()
	if len(haStorage) == 0 {
		return ""
	}
	return reflect.ValueOf(haStorage).MapKeys()[0].String()
}

// GetVersion returns the version of Vault
func (spec *VaultSpec) GetVersion() (*semver.Version, error) {
	ref, err := reference.ParseAnyReference(spec.Image)
	if err != nil {
		return nil, fmt.Errorf("parsing image ref for Vault version: %w", err)
	}

	taggedRef, ok := ref.(reference.Tagged)
	if !ok {
		return nil, errors.New("Vault image ref does not have a tag")
	}

	return semver.NewVersion(taggedRef.Tag())
}

// GetServiceAccount returns the Kubernetes Service Account to use for Vault
func (spec *VaultSpec) GetServiceAccount() string {
	if spec.ServiceAccount != "" {
		return spec.ServiceAccount
	}
	return "default"
}

// HasStorageHAEnabled detects if the ha_enabled field is set to true in Vault's storage stanza
func (spec *VaultSpec) HasStorageHAEnabled() bool {
	storageType := spec.GetStorageType()
	storage := spec.getStorage()
	storageSpecs := cast.ToStringMap(storage[storageType])
	// In Consul HA is always enabled
	return storageType == "consul" || storageType == "raft" || cast.ToBool(storageSpecs["ha_enabled"])
}

// IsTLSDisabled returns if Vault's TLS should be disabled
func (spec *VaultSpec) IsTLSDisabled() bool {
	listener := spec.getListener()
	tcp := cast.ToStringMap(listener["tcp"])
	return cast.ToBool(tcp["tls_disable"])
}

// IsTelemetryUnauthenticated returns if Vault's telemetry endpoint can be accessed publicly
func (spec *VaultSpec) IsTelemetryUnauthenticated() bool {
	listener := spec.getListener()
	tcp := cast.ToStringMap(listener["tcp"])
	telemetry := cast.ToStringMap(tcp["telemetry"])
	return cast.ToBool(telemetry["unauthenticated_metrics_access"])
}

// GetAPIScheme returns if Vault's API address should be called on http or https
func (spec *VaultSpec) GetAPIScheme() string {
	if spec.IsTLSDisabled() {
		return "http"
	}
	return "https"
}

// GetTLSExpiryThreshold returns the Vault TLS certificate expiration threshold
func (spec *VaultSpec) GetTLSExpiryThreshold() time.Duration {
	if spec.TLSExpiryThreshold == "" {
		return time.Hour * 168
	}
	duration, err := time.ParseDuration(spec.TLSExpiryThreshold)
	if err != nil {
		log.Error(err, "using default threshold due to parse error", "tlsExpiryThreshold", spec.TLSExpiryThreshold)
		return time.Hour * 168
	}
	return duration
}

func (spec *VaultSpec) getListener() map[string]interface{} {
	config := spec.GetVaultConfig()
	return cast.ToStringMap(config["listener"])
}

// GetVaultImage returns the Vault image to use
func (spec *VaultSpec) GetVaultImage() string {
	if spec.Image == "" {
		return "hashicorp/vault:latest"
	}
	return spec.Image
}

// GetBankVaultsImage returns the bank-vaults image to use
func (spec *VaultSpec) GetBankVaultsImage() string {
	if spec.BankVaultsImage == "" {
		return DefaultBankVaultsImage
	}
	return spec.BankVaultsImage
}

// GetStatsDImage returns the StatsD image to use
func (spec *VaultSpec) GetStatsDImage() string {
	if spec.StatsDImage == "" {
		return "prom/statsd-exporter:latest"
	}
	return spec.StatsDImage
}

// GetVeleroFsfreezeImage returns the Velero Fsreeze image to use
func (spec *VaultSpec) GetVeleroFsfreezeImage() string {
	if spec.VeleroFsfreezeImage == "" {
		return "ubuntu:bionic"
	}
	return spec.VeleroFsfreezeImage
}

// GetVolumeClaimTemplates fixes the "status diff" in PVC templates
func (spec *VaultSpec) GetVolumeClaimTemplates() []v1.PersistentVolumeClaim {
	var pvcs []v1.PersistentVolumeClaim
	for _, pvc := range spec.VolumeClaimTemplates {
		pvcs = append(pvcs, v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        pvc.Name,
				Labels:      pvc.Labels,
				Annotations: pvc.Annotations,
			},
			Spec: pvc.Spec,
		})
	}
	return pvcs
}

// GetWatchedSecretsLabels returns the set of labels for secrets to watch in the vault namespace
func (spec *VaultSpec) GetWatchedSecretsLabels() []map[string]string {
	if spec.WatchedSecretsLabels == nil {
		spec.WatchedSecretsLabels = []map[string]string{}
	}

	return spec.WatchedSecretsLabels
}

// GetWatchedSecretsAnnotations returns the set of annotations for secrets to watch in the vault namespace
func (spec *VaultSpec) GetWatchedSecretsAnnotations() []map[string]string {
	if spec.WatchedSecretsAnnotations == nil {
		spec.WatchedSecretsAnnotations = []map[string]string{}
	}

	return spec.WatchedSecretsAnnotations
}

// GetAnnotations returns the Common Annotations
func (spec *VaultSpec) GetAnnotations() map[string]string {
	if spec.Annotations == nil {
		spec.Annotations = map[string]string{}
	}

	return spec.Annotations
}

// GetAPIPortName returns the main Vault port name based on Istio and TLS settings
func (spec *VaultSpec) GetAPIPortName() string {
	portName := "api-port"
	if spec.IstioEnabled {
		if spec.IsTLSDisabled() {
			return "http-" + portName
		}
		return "https-" + portName
	}
	return portName
}

// GetVaultLabels returns the Vault Pod, Secret and ConfigMap Labels
func (spec *VaultSpec) GetVaultLabels() map[string]string {
	if spec.VaultLabels == nil {
		spec.VaultLabels = map[string]string{}
	}

	return spec.VaultLabels
}

// GetVaultConfigurerLabels returns the Vault Configurer Pod Labels
func (spec *VaultSpec) GetVaultConfigurerLabels() map[string]string {
	if spec.VaultConfigurerLabels == nil {
		spec.VaultConfigurerLabels = map[string]string{}
	}

	return spec.VaultConfigurerLabels
}

// GetVaultAnnotations returns the Vault Pod , Secret and ConfigMap Annotations
func (spec *VaultSpec) GetVaultAnnotations() map[string]string {
	if spec.VaultAnnotations == nil {
		spec.VaultAnnotations = map[string]string{}
	}

	return spec.VaultAnnotations
}

// GetVaultConfigurerAnnotations returns the Vault Configurer Pod Annotations
func (spec *VaultSpec) GetVaultConfigurerAnnotations() map[string]string {
	if spec.VaultConfigurerAnnotations == nil {
		spec.VaultConfigurerAnnotations = map[string]string{}
	}

	return spec.VaultConfigurerAnnotations
}

// GetFluentDImage returns the FluentD image to use
func (spec *VaultSpec) GetFluentDImage() string {
	if spec.FluentDImage == "" {
		return "fluent/fluentd:edge"
	}
	return spec.FluentDImage
}

// GetFluentDConfMountPath returns the mount path for the fluent.conf
func (spec *VaultSpec) GetFluentDConfMountPath() string {
	if spec.FluentDConfLocation == "" {
		return "/fluentd/etc"
	}
	return spec.FluentDConfLocation
}

func (spec *VaultSpec) GetConfigPath() string {
	if spec.ConfigPath == "" {
		return "/vault/config"
	}
	return spec.ConfigPath
}

// IsFluentDEnabled returns true if fluentd sidecar is to be deployed
func (spec *VaultSpec) IsFluentDEnabled() bool {
	return spec.FluentDEnabled
}

// IsStatsDDisabled returns false if statsd sidecar is to be deployed
func (spec *VaultSpec) IsStatsDDisabled() bool {
	return spec.StatsDDisabled
}

// ExternalConfigJSON returns the ExternalConfig field as a JSON string
func (spec *VaultSpec) ExternalConfigJSON() []byte {
	return spec.ExternalConfig.Raw
}

// IsAutoUnseal checks if auto-unseal is configured
func (spec *VaultSpec) IsAutoUnseal() bool {
	config := spec.GetVaultConfig()
	_, ok := config["seal"]
	return ok
}

// IsRaftStorage checks if raft storage is configured
func (spec *VaultSpec) IsRaftStorage() bool {
	return spec.GetStorageType() == "raft"
}

// IsRaftHAStorage checks if raft ha_storage is configured
func (spec *VaultSpec) IsRaftHAStorage() bool {
	return spec.GetStorageType() != "raft" && spec.GetHAStorageType() == "raft"
}

// IsRaftBootstrapFollower checks if this cluster should be considered the bootstrap follower.
func (spec *VaultSpec) IsRaftBootstrapFollower() bool {
	return spec.RaftLeaderAddress != "" && spec.RaftLeaderAddress != "self"
}

// VaultStatus defines the observed state of Vault
type VaultStatus struct {
	// Important: Run "make generate-code" to regenerate code after modifying this file
	Nodes      []string                `json:"nodes"`
	Leader     string                  `json:"leader"`
	Conditions []v1.ComponentCondition `json:"conditions,omitempty"`
}

// UnsealOptions represents the common options to all unsealing backends
type UnsealOptions struct {
	PreFlightChecks *bool `json:"preFlightChecks,omitempty"`
	StoreRootToken  *bool `json:"storeRootToken,omitempty"`
	SecretThreshold *uint `json:"secretThreshold,omitempty"`
	SecretShares    *uint `json:"secretShares,omitempty"`
}

// UnsealConfig represents the UnsealConfig field of a VaultSpec Kubernetes object
type UnsealConfig struct {
	Options    UnsealOptions          `json:"options,omitempty"`
	Kubernetes KubernetesUnsealConfig `json:"kubernetes,omitempty"`
	Google     *GoogleUnsealConfig    `json:"google,omitempty"`
	Alibaba    *AlibabaUnsealConfig   `json:"alibaba,omitempty"`
	Azure      *AzureUnsealConfig     `json:"azure,omitempty"`
	AWS        *AWSUnsealConfig       `json:"aws,omitempty"`
	OCI        *OCIUnsealConfig       `json:"oci,omitempty"`
	Vault      *VaultUnsealConfig     `json:"vault,omitempty"`
	HSM        *HSMUnsealConfig       `json:"hsm,omitempty"`
}

// ToArgs returns the UnsealConfig as and argument array for bank-vaults
func (usc *UnsealConfig) ToArgs(vault *Vault) []string {
	args := []string{}

	// PreFlightChecks is true by default
	if usc.Options.PreFlightChecks != nil && !*usc.Options.PreFlightChecks {
		args = append(args, "--pre-flight-checks=false")
	}
	// StoreRootToken is true by default
	if usc.Options.StoreRootToken != nil && !*usc.Options.StoreRootToken {
		args = append(args, "--store-root-token=false")
	}

	// SecretShares is 5 by default
	if usc.Options.SecretShares != nil && *usc.Options.SecretShares > 0 {
		args = append(args, "--secret-shares", fmt.Sprint(*usc.Options.SecretShares))
	}

	// SecretThreshold is 3 by default
	if usc.Options.SecretThreshold != nil && *usc.Options.SecretThreshold > 0 {
		args = append(args, "--secret-threshold", fmt.Sprint(*usc.Options.SecretThreshold))
	}

	if usc.Google != nil {
		args = append(args,
			"--mode",
			"google-cloud-kms-gcs",
			"--google-cloud-kms-key-ring",
			usc.Google.KMSKeyRing,
			"--google-cloud-kms-crypto-key",
			usc.Google.KMSCryptoKey,
			"--google-cloud-kms-location",
			usc.Google.KMSLocation,
			"--google-cloud-kms-project",
			usc.Google.KMSProject,
			"--google-cloud-storage-bucket",
			usc.Google.StorageBucket,
		)
	} else if usc.Azure != nil {
		args = append(args,
			"--mode",
			"azure-key-vault",
			"--azure-key-vault-name",
			usc.Azure.KeyVaultName,
		)
	} else if usc.OCI != nil {
		args = append(args,
			"--mode",
			"oci",
			"--oci-key-ocid",
			usc.OCI.KeyOCID,
			"--oci-cryptographic-endpoint",
			usc.OCI.CryptographicEndpoint,
			"--oci-bucket-namespace",
			usc.OCI.BucketNamespace,
			"--oci-bucket-name",
			usc.OCI.BucketName,
			"--oci-bucket-prefix",
			usc.OCI.BucketPrefix,
		)
	} else if usc.AWS != nil {
		args = append(args,
			"--mode",
			"aws-kms-s3",
			"--aws-kms-key-id",
			usc.AWS.KMSKeyID,
			"--aws-kms-region",
			usc.AWS.KMSRegion,
			"--aws-s3-bucket",
			usc.AWS.S3Bucket,
			"--aws-s3-prefix",
			usc.AWS.S3Prefix,
			"--aws-s3-region",
			usc.AWS.S3Region,
			"--aws-s3-sse-algo",
			usc.AWS.S3SSE,
		)

		if usc.AWS.KMSEncryptionContext != "" {
			args = append(args,
				"--aws-kms-encryption-context",
				usc.AWS.KMSEncryptionContext,
			)
		}
	} else if usc.Alibaba != nil {
		args = append(args,
			"--mode",
			"alibaba-kms-oss",
			"--alibaba-kms-region",
			usc.Alibaba.KMSRegion,
			"--alibaba-kms-key-id",
			usc.Alibaba.KMSKeyID,
			"--alibaba-oss-endpoint",
			usc.Alibaba.OSSEndpoint,
			"--alibaba-oss-bucket",
			usc.Alibaba.OSSBucket,
			"--alibaba-oss-prefix",
			usc.Alibaba.OSSPrefix,
		)
	} else if usc.Vault != nil {
		args = append(args,
			"--mode",
			"vault",
			"--vault-addr",
			usc.Vault.Address,
			"--vault-unseal-keys-path",
			usc.Vault.UnsealKeysPath,
		)

		if usc.Vault.Token != "" {
			args = append(args,
				"--vault-token",
				usc.Vault.Token,
			)
		} else if usc.Vault.TokenPath != "" {
			args = append(args,
				"--vault-token-path",
				usc.Vault.TokenPath,
			)
		} else if usc.Vault.Role != "" {
			args = append(args,
				"--vault-role",
				usc.Vault.Role,
				"--vault-auth-path",
				usc.Vault.AuthPath,
			)
		}
	} else if usc.HSM != nil {
		mode := "hsm"
		if usc.Kubernetes.SecretNamespace != "" && usc.Kubernetes.SecretName != "" {
			mode = "hsm-k8s"
		}

		args = append(args,
			"--mode",
			mode,
			"--hsm-module-path",
			usc.HSM.ModulePath,
			"--hsm-slot-id",
			fmt.Sprint(usc.HSM.SlotID),
			"--hsm-key-label",
			usc.HSM.KeyLabel,
		)

		if usc.HSM.Pin != "" {
			args = append(args,
				"--hsm-pin",
				usc.HSM.Pin,
			)
		}

		if usc.HSM.TokenLabel != "" {
			args = append(args,
				"--hsm-token-label",
				usc.HSM.TokenLabel,
			)
		}

		if mode == "hsm-k8s" {
			var secretLabels []string
			for k, v := range vault.LabelsForVault() {
				secretLabels = append(secretLabels, k+"="+v)
			}

			sort.Strings(secretLabels)

			args = append(args,
				"--k8s-secret-namespace",
				usc.Kubernetes.SecretNamespace,
				"--k8s-secret-name",
				usc.Kubernetes.SecretName,
				"--k8s-secret-labels",
				strings.Join(secretLabels, ","),
			)
		}
	} else {
		secretNamespace := vault.Namespace
		if usc.Kubernetes.SecretNamespace != "" {
			secretNamespace = usc.Kubernetes.SecretNamespace
		}

		secretName := vault.Name + "-unseal-keys"
		if usc.Kubernetes.SecretName != "" {
			secretName = usc.Kubernetes.SecretName
		}

		var secretLabels []string
		for k, v := range vault.LabelsForVault() {
			secretLabels = append(secretLabels, k+"="+v)
		}

		sort.Strings(secretLabels)

		args = append(args,
			"--mode",
			"k8s",
			"--k8s-secret-namespace",
			secretNamespace,
			"--k8s-secret-name",
			secretName,
			"--k8s-secret-labels",
			strings.Join(secretLabels, ","),
		)
	}

	return args
}

// HSMDaemonNeeded returns if the unsealing mechanism needs a HSM Daemon present
func (usc *UnsealConfig) HSMDaemonNeeded() bool {
	return usc.HSM != nil && usc.HSM.Daemon
}

// KubernetesUnsealConfig holds the parameters for Kubernetes based unsealing
type KubernetesUnsealConfig struct {
	SecretNamespace string `json:"secretNamespace,omitempty"`
	SecretName      string `json:"secretName,omitempty"`
}

// GoogleUnsealConfig holds the parameters for Google KMS based unsealing
type GoogleUnsealConfig struct {
	KMSKeyRing    string `json:"kmsKeyRing"`
	KMSCryptoKey  string `json:"kmsCryptoKey"`
	KMSLocation   string `json:"kmsLocation"`
	KMSProject    string `json:"kmsProject"`
	StorageBucket string `json:"storageBucket"`
}

// AlibabaUnsealConfig holds the parameters for Alibaba Cloud KMS based unsealing
//
//	--alibaba-kms-region eu-central-1 --alibaba-kms-key-id 9d8063eb-f9dc-421b-be80-15d195c9f148 --alibaba-oss-endpoint oss-eu-central-1.aliyuncs.com --alibaba-oss-bucket bank-vaults
type AlibabaUnsealConfig struct {
	KMSRegion   string `json:"kmsRegion"`
	KMSKeyID    string `json:"kmsKeyId"`
	OSSEndpoint string `json:"ossEndpoint"`
	OSSBucket   string `json:"ossBucket"`
	OSSPrefix   string `json:"ossPrefix"`
}

// AzureUnsealConfig holds the parameters for Azure Key Vault based unsealing
type AzureUnsealConfig struct {
	KeyVaultName string `json:"keyVaultName"`
}

// AWSUnsealConfig holds the parameters for AWS KMS based unsealing
type AWSUnsealConfig struct {
	KMSKeyID             string `json:"kmsKeyId"`
	KMSRegion            string `json:"kmsRegion,omitempty"`
	KMSEncryptionContext string `json:"kmsEncryptionContext,omitempty"`
	S3Bucket             string `json:"s3Bucket"`
	S3Prefix             string `json:"s3Prefix"`
	S3Region             string `json:"s3Region,omitempty"`
	S3SSE                string `json:"s3SSE,omitempty"`
}

// OCIUnsealConfig holds the parameters for Oracle Cloud Infrastructure based unsealing
type OCIUnsealConfig struct {
	KeyOCID               string `json:"keyOCID"`
	CryptographicEndpoint string `json:"cryptographicEndpoint"`
	BucketName            string `json:"bucketName"`
	BucketNamespace       string `json:"bucketNamespace,omitempty"`
	BucketPrefix          string `json:"bucketPrefix,omitempty"`
}

// VaultUnsealConfig holds the parameters for remote Vault based unsealing
type VaultUnsealConfig struct {
	Address        string `json:"address"`
	UnsealKeysPath string `json:"unsealKeysPath"`
	Role           string `json:"role,omitempty"`
	AuthPath       string `json:"authPath,omitempty"`
	TokenPath      string `json:"tokenPath,omitempty"`
	Token          string `json:"token,omitempty"`
}

// HSMUnsealConfig holds the parameters for remote HSM based unsealing
type HSMUnsealConfig struct {
	Daemon     bool   `json:"daemon,omitempty"`
	ModulePath string `json:"modulePath"`
	SlotID     uint   `json:"slotId,omitempty"`
	TokenLabel string `json:"tokenLabel,omitempty"`
	// +optional
	Pin      string `json:"pin"`
	KeyLabel string `json:"keyLabel"`
}

// CredentialsConfig configuration for a credentials file provided as a secret
type CredentialsConfig struct {
	Env        string `json:"env"`
	Path       string `json:"path"`
	SecretName string `json:"secretName"`
}

// Resources holds different container's ResourceRequirements
type Resources struct {
	Vault              *v1.ResourceRequirements `json:"vault,omitempty"`
	BankVaults         *v1.ResourceRequirements `json:"bankVaults,omitempty"`
	HSMDaemon          *v1.ResourceRequirements `json:"hsmDaemon,omitempty"`
	PrometheusExporter *v1.ResourceRequirements `json:"prometheusExporter,omitempty"`
	FluentD            *v1.ResourceRequirements `json:"fluentd,omitempty"`
}

// Ingress specification for the Vault cluster
type Ingress struct {
	Annotations map[string]string `json:"annotations,omitempty"`
	Spec        netv1.IngressSpec `json:"spec,omitempty"`
}

// +genclient
// +genclient:noStatus
// +kubebuilder:object:root=true

// Vault is the Schema for the vaults API
type Vault struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VaultSpec   `json:"spec,omitempty"`
	Status VaultStatus `json:"status,omitempty"`
}

// ConfigJSON returns the Config field as a JSON string
func (vault *Vault) ConfigJSON() ([]byte, error) {
	config := map[string]interface{}{}

	err := json.Unmarshal(vault.Spec.Config.Raw, &config)
	if err != nil {
		return nil, err
	}

	if vault.Spec.ServiceRegistrationEnabled && vault.Spec.HasHAStorage() {
		serviceRegistration := map[string]interface{}{
			"service_registration": map[string]interface{}{
				"kubernetes": map[string]string{
					"namespace": vault.Namespace,
				},
			},
		}

		if err := mergo.Merge(&config, serviceRegistration); err != nil {
			return nil, err
		}
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	return configJSON, nil
}

// GetIngress the Ingress configuration for Vault if any
func (vault *Vault) GetIngress() *Ingress {
	if vault.Spec.Ingress != nil {
		// Add the Vault Service as the backend if no rules are specified and there is no default backend
		if len(vault.Spec.Ingress.Spec.Rules) == 0 && vault.Spec.Ingress.Spec.DefaultBackend == nil {
			vault.Spec.Ingress.Spec.DefaultBackend = &netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: vault.Name,
					Port: netv1.ServiceBackendPort{
						Number: 8200,
					},
				},
			}
		}

		if vault.Spec.Ingress.Annotations == nil {
			vault.Spec.Ingress.Annotations = map[string]string{}
		}

		// If TLS is enabled add the Ingress TLS backend annotations
		if !vault.Spec.IsTLSDisabled() {
			// Supporting the NGINX ingress controller with TLS backends
			// https://kubernetes.github.io/ingress-nginx/user-guide/nginx-configuration/annotations/#backend-protocol
			vault.Spec.Ingress.Annotations["nginx.ingress.kubernetes.io/backend-protocol"] = "HTTPS"

			// Supporting the Traefik ingress controller with TLS backends
			// https://docs.traefik.io/configuration/backends/kubernetes/#tls-communication-between-traefik-and-backend-pods
			vault.Spec.Ingress.Annotations["ingress.kubernetes.io/protocol"] = "https"

			// Supporting the HAProxy ingress controller with TLS backends
			// https://github.com/jcmoraisjr/haproxy-ingress#secure-backend
			vault.Spec.Ingress.Annotations["ingress.kubernetes.io/secure-backends"] = "true"
		}

		return vault.Spec.Ingress
	}

	return nil
}

// LabelsForVault returns the labels for selecting the resources
// belonging to the given vault CR name.
func (vault *Vault) LabelsForVault() map[string]string {
	return map[string]string{"app.kubernetes.io/name": "vault", "vault_cr": vault.Name}
}

// LabelsForVaultConfigurer returns the labels for selecting the resources
// belonging to the given vault CR name.
func (vault *Vault) LabelsForVaultConfigurer() map[string]string {
	return map[string]string{"app.kubernetes.io/name": "vault-configurator", "vault_cr": vault.Name}
}

// AsOwnerReference returns this Vault instance as an OwnerReference
func (vault *Vault) AsOwnerReference() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: vault.APIVersion,
		Kind:       vault.Kind,
		Name:       vault.Name,
		UID:        vault.UID,
		Controller: ptr.To(true),
	}
}

// +kubebuilder:object:root=true

// VaultList contains a list of Vault
type VaultList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Vault `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Vault{}, &VaultList{})
}
