import * as awsx from "@pulumi/awsx";
import * as eks from "@pulumi/eks";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";
import * as echoserver from "./echoserver";
import * as iam from "./iam";
import * as nginx from "./nginx";
import * as utils from "./utils";

// Define name, and tags to use on the cluster and any taggable resource
// under management.
const projectName = pulumi.getProject();
const tags = { "project": "PulumiEKSUpgrade", "org": "KubeTeam" };

// Allocate a new VPC with custom settings, and a public & private subnet per AZ.
const vpc = new awsx.ec2.Vpc(`${projectName}`, {
    cidrBlock: "172.16.0.0/16",
    subnets: [{ type: "public", tags: tags }, { type: "private", tags: tags }],
});

// Export VPC ID and Subnets.
export const vpcId = vpc.id;
export const allVpcSubnets = vpc.privateSubnetIds.concat(vpc.publicSubnetIds);

// Create 3 IAM Roles and InstanceProfiles to use on each of the 3 nodegroups.
const roles = iam.createRoles(projectName, 3);
const instanceProfiles = iam.createInstanceProfiles(projectName, roles);

// Create an EKS cluster with no default node group, IAM roles for two node groups
// logging, private subnets for the nodegroup workers, and resource tags.
const myCluster = new eks.Cluster(`${projectName}`, {
    version: "1.13",
    vpcId: vpcId,
    subnetIds: allVpcSubnets,
    nodeAssociatePublicIpAddress: false,
    skipDefaultNodeGroup: true,
    deployDashboard: false,
    instanceRoles: roles,
    enabledClusterLogTypes: ["api", "audit", "authenticator",
        "controllerManager", "scheduler"],
    tags: tags,
    clusterSecurityGroupTags: { "myClusterSecurityGroupTag": "true" },
    nodeSecurityGroupTags: { "myNodeSecurityGroupTag": "true" },
});
export const kubeconfig = myCluster.kubeconfig;
export const clusterName = myCluster.core.cluster.name;

// Create a standard node group of t2.medium workers.
const ngStandard = utils.createNodeGroup(`${projectName}-ng-standard`,
    // "ami-03a55127c613349a7", // k8s v1.13.7 in us-west-2
    "ami-0485258c2d1c3608f", // k8s v1.13.7 in us-east-2
    "t2.medium",
    3,
    myCluster,
    instanceProfiles[0],
);

// Create a 2xlarge node group of t3.2xlarge workers, with taints on the nodes.
// This node group is dedicated for the NGINX Ingress Controller.
const ng2xlarge = utils.createNodeGroup(`${projectName}-ng-2xlarge`,
    // "ami-0355c210cb3f58aa2", // k8s v1.12.7 in us-west-2
    "ami-0485258c2d1c3608f", // k8s v1.13.7 in us-east-2
    "t3.2xlarge",
    3,
    myCluster,
    instanceProfiles[1],
    {"nginx": { value: "true", effect: "NoSchedule"}},
);

// Create a Namespace for NGINX Ingress Controller and the echoserver workload.
const namespace = new k8s.core.v1.Namespace("apps", undefined, { provider: myCluster.provider });
export const namespaceName = namespace.metadata.apply(m => m.name);

// Deploy the NGINX Ingress Controller, preferably on the t3.2xlarge node group.
const nginxService = nginx.create("nginx-ing-cntlr",
    3,
    namespaceName,
    "my-nginx-class",
    myCluster,
    ["t3.2xlarge"],
);
export const nginxServiceUrl = nginxService.status.loadBalancer.ingress[0].hostname;

// Deploy the echoserver Workload on the standard node group.
const echoserverDeployment = echoserver.create("echoserver",
    3,
    namespaceName,
    "my-nginx-class",
    myCluster.provider,
);
