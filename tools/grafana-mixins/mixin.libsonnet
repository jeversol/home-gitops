local kubernetes = import "kubernetes-mixin/mixin.libsonnet";
local node = import "node-mixin/mixin.libsonnet";

local rawMixins = kubernetes + node + {
  _config+:: {
    kubeStateMetricsSelector: 'job="kube-state-metrics"',
    cadvisorSelector: 'job="kubelet"',
    kubeletSelector: 'job="kubelet"',
    kubeApiserverSelector: 'job="apiserver"',
    nodeExporterSelector: 'job="node-exporter"',

    grafanaK8s+:: {
      dashboardTags: ['kubernetes', 'mixin'],
      dashboardNamePrefix: '',
      grafanaTimezone: 'browser',
    },
  },
};

rawMixins