package cli

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type connectionTestClientConfig struct {
	clientConfig
	config *rest.Config
	err    error
}

func (c *connectionTestClientConfig) ClientConfig() (*rest.Config, error) {
	return c.config, c.err
}

func captureConnectionStderr(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	original := os.Stderr
	os.Stderr = writer
	defer func() {
		os.Stderr = original
		_ = writer.Close()
	}()
	run()
	require.NoError(t, writer.Close())
	output, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(output)
}

func TestWithConnectionLogging(t *testing.T) {
	configErr := errors.New("invalid kubeconfig")
	authenticatedURL := &url.URL{
		Scheme:   "https",
		Host:     "cluster.example:6443",
		Path:     "/api",
		User:     url.UserPassword("test-user", "test-pass"),
		RawQuery: "token=private",
		Fragment: "private",
	}
	for _, tt := range []struct {
		name   string
		host   string
		err    error
		output string
	}{
		{name: "URL", host: "https://cluster.example:6443", output: "Connecting to Kubernetes server https://cluster.example:6443\n"},
		{name: "IP", host: "192.0.2.1", output: "Connecting to Kubernetes server 192.0.2.1\n"},
		{name: "IPv6", host: "https://[2001:db8::1]:6443", output: "Connecting to Kubernetes server https://[2001:db8::1]:6443\n"},
		{name: "credentials", host: authenticatedURL.String(), output: "Connecting to Kubernetes server https://cluster.example:6443/api\n"},
		{name: "invalid URL", host: "https://cluster.example/%invalid", output: "Connecting to Kubernetes server <invalid server URL>\n"},
		{name: "configuration error", err: configErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &rest.Config{Host: tt.host}
			delegate := &connectionTestClientConfig{config: config, err: tt.err}
			wrapped := WithConnectionLogging(delegate)
			require.Same(t, wrapped, WithConnectionLogging(wrapped))
			output := captureConnectionStderr(t, func() {
				resolved, err := wrapped.ClientConfig()
				if tt.err != nil {
					require.ErrorIs(t, err, tt.err)
					require.Nil(t, resolved)
				} else {
					require.NoError(t, err)
					require.Same(t, config, resolved)
					require.Equal(t, tt.host, resolved.Host)
				}
			})
			require.Equal(t, tt.output, output)
		})
	}
}

func TestAddKubectlFlagsReportsResolvedServer(t *testing.T) {
	config := clientcmdapi.Config{
		CurrentContext: "default",
		Contexts: map[string]*clientcmdapi.Context{
			"default": {Cluster: "default", AuthInfo: "test", Namespace: "argocd"},
			"other":   {Cluster: "other", AuthInfo: "test", Namespace: "other"},
		},
		Clusters: map[string]*clientcmdapi.Cluster{
			"default": {Server: "https://default.example:6443"},
			"other":   {Server: "https://other.example:6443"},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"test": {Token: "test-token"},
		},
	}
	configPath := filepath.Join(t.TempDir(), "config")
	require.NoError(t, clientcmd.WriteToFile(config, configPath))
	for _, tt := range []struct {
		name   string
		flags  []string
		server string
	}{
		{name: "current context", server: "https://default.example:6443"},
		{name: "context override", flags: []string{"--context", "other"}, server: "https://other.example:6443"},
		{name: "server override", flags: []string{"--server", "https://override.example:6443"}, server: "https://override.example:6443"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet(t.Name(), pflag.ContinueOnError)
			client := AddKubectlFlagsToSet(flags)
			require.NoError(t, flags.Parse(append([]string{"--kubeconfig", configPath}, tt.flags...)))
			raw, err := client.RawConfig()
			require.NoError(t, err)
			require.Equal(t, config.CurrentContext, raw.CurrentContext)
			namespace, _, err := client.Namespace()
			require.NoError(t, err)
			require.NotEmpty(t, namespace)
			output := captureConnectionStderr(t, func() {
				resolved, err := client.ClientConfig()
				require.NoError(t, err)
				require.Equal(t, tt.server, resolved.Host)
			})
			require.Equal(t, "Connecting to Kubernetes server "+tt.server+"\n", output)
		})
	}
}
