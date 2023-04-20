# Deploying route-monitor-operator

## Hypershift

For managed clusters, route-monitor-operator is deployed via a [package-operator](https://package-operator.run/) package, using the [hypershift integration](https://package-operator.run/docs/concepts/hypershift-integration/) of package-operator.

The resources included in the package are contained in the [packaging](../packaging/) directory and its phases are defined in the [package manifest](../packaging/manifest.yaml).

Building the package can be done by running `make package`. This target will install the kubectl-package plugin locally (if needed), build the route-monitor-operator package and push it to quay.io/app-sre/route-monitor-operator-hs-package, tagged with the same image tag as the operator itself.

Deploying route-monitor-operator and setting up monitoring can be done with a pair of [ACM governance policies](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/2.7/html/governance/governance#kubernetes-configuration-policy-controller): one to install the route-monitor-operator package and configure the api clusterurlmonitor and another to configure the console routemonitor. These policies are deployed to the hypershift clusters via hive selectorsyncsets:

    - hive selectorsyncset
    | - ACM governance policy configuring hosted control plane namespaces on management clusters
      | - package.package-operator.run resource that installs route-monitor-operator on the hosted cluster
      | - clusterurlmonitor.monitoring.openshift.io resource that monitors the hosted cluster's api availability

    - hive selectorsyncset
    | - ACM governance policy configuring managed clusters
      | - routemonitor.monitoring.openshift.io resource that monitors the hosted cluster's console availability
