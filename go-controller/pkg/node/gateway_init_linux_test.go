// +build linux

package node

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"syscall"

	"github.com/urfave/cli/v2"
	kapi "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/config"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/factory"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/kube"
	ovntest "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/testing"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/types"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/vishvananda/netlink"

	egressfirewallfake "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/crd/egressfirewall/v1/apis/clientset/versioned/fake"
	egressipfake "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/crd/egressip/v1/apis/clientset/versioned/fake"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func startGateway(gw Gateway, wf factory.NodeWatchFactory) {
	wf.AddServiceHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			svc := obj.(*kapi.Service)
			gw.AddService(svc)
		},
		UpdateFunc: func(old, new interface{}) {
			oldSvc := old.(*kapi.Service)
			newSvc := new.(*kapi.Service)
			gw.UpdateService(oldSvc, newSvc)
		},
		DeleteFunc: func(obj interface{}) {
			svc := obj.(*kapi.Service)
			gw.DeleteService(svc)
		},
	}, gw.SyncServices)
}

func setupNodeAccessBridgeTest(fexec *ovntest.FakeExec, nodeName, brLocalnetMAC, mtu string) {
	gwPortMac := util.IPAddrToHWAddr(net.ParseIP(types.V4NodeLocalNATSubnetNextHop)).String()

	fexec.AddFakeCmdsNoOutputNoError([]string{
		"ovs-vsctl --timeout=15 --may-exist add-br br-local",
	})
	fexec.AddFakeCmd(&ovntest.ExpectedCmd{
		Cmd:    "ovs-vsctl --timeout=15 --if-exists get interface br-local mac_in_use",
		Output: brLocalnetMAC,
	})
	fexec.AddFakeCmdsNoOutputNoError([]string{
		"ovs-vsctl --timeout=15 set bridge br-local other-config:hwaddr=" + brLocalnetMAC,
	})
	fexec.AddFakeCmd(&ovntest.ExpectedCmd{
		Cmd:    "ovs-vsctl --timeout=15 --if-exists get Open_vSwitch . external_ids:ovn-bridge-mappings",
		Output: types.PhysicalNetworkName + ":breth0",
	})
	fexec.AddFakeCmdsNoOutputNoError([]string{
		"ovs-vsctl --timeout=15 set Open_vSwitch . external_ids:ovn-bridge-mappings=" + types.PhysicalNetworkName + ":breth0" + "," + types.LocalNetworkName + ":br-local",
	})
	fexec.AddFakeCmdsNoOutputNoError([]string{
		"ovs-vsctl --timeout=15 --may-exist add-port br-local " + localnetGatewayNextHopPort +
			" -- set interface " + localnetGatewayNextHopPort + " type=internal mtu_request=" + mtu + " mac=" + strings.ReplaceAll(gwPortMac, ":", "\\:"),
	})
}

func shareGatewayInterfaceTest(app *cli.App, testNS ns.NetNS,
	eth0Name, eth0MAC, eth0IP, eth0GWIP, eth0CIDR string, gatewayVLANID uint) {
	const mtu string = "1234"
	const clusterCIDR string = "10.1.0.0/16"
	app.Action = func(ctx *cli.Context) error {
		const (
			nodeName      string = "node1"
			brNextHopIp   string = types.V4NodeLocalNATSubnetNextHop
			systemID      string = "cb9ec8fa-b409-4ef3-9f42-d9283c47aac6"
			nodeSubnet    string = "10.1.1.0/24"
			brLocalnetMAC string = "11:22:33:44:55:66"
		)

		brNextHopCIDR := fmt.Sprintf("%s/%d", brNextHopIp, types.V4NodeLocalNATSubnetPrefix)
		fexec := ovntest.NewLooseCompareFakeExec()
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd: "ovs-vsctl --timeout=15 -- port-to-br eth0",
			Err: fmt.Errorf(""),
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd: "ovs-vsctl --timeout=15 -- br-exists eth0",
			Err: fmt.Errorf(""),
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd: "ovs-vsctl --timeout=15 -- --may-exist add-br breth0 -- br-set-external-id breth0 bridge-id breth0 -- br-set-external-id breth0 bridge-uplink eth0 -- set bridge breth0 fail-mode=standalone other_config:hwaddr=" + eth0MAC + " -- --may-exist add-port breth0 eth0 -- set port eth0 other-config:transient=true",
			Action: func() error {
				return testNS.Do(func(ns.NetNS) error {
					defer GinkgoRecover()

					// Create breth0 as a dummy link
					err := netlink.LinkAdd(&netlink.Dummy{
						LinkAttrs: netlink.LinkAttrs{
							Name:         "br" + eth0Name,
							HardwareAddr: ovntest.MustParseMAC(eth0MAC),
						},
					})
					Expect(err).NotTo(HaveOccurred())
					_, err = netlink.LinkByName("br" + eth0Name)
					Expect(err).NotTo(HaveOccurred())
					return nil
				})
			},
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get interface breth0 mac_in_use",
			Output: eth0MAC,
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-vsctl --timeout=15 set bridge breth0 other-config:hwaddr=" + eth0MAC,
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get Open_vSwitch . external_ids:ovn-bridge-mappings",
			Output: "",
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-vsctl --timeout=15 set Open_vSwitch . external_ids:ovn-bridge-mappings=" + types.PhysicalNetworkName + ":breth0",
		})

		setupNodeAccessBridgeTest(fexec, nodeName, brLocalnetMAC, mtu)

		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get Open_vSwitch . external_ids:system-id",
			Output: systemID,
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 wait-until Interface patch-breth0_node1-to-br-int ofport>0 -- get Interface patch-breth0_node1-to-br-int ofport",
			Output: "5",
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 get interface eth0 ofport",
			Output: "7",
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-ofctl -O OpenFlow13 replace-flows breth0 -",
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=10, table=0, in_port=7, dl_dst=" + eth0MAC + ", actions=output:5,output:LOCAL",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=100, in_port=5, ip, actions=ct(commit, zone=64000), output:7",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=50, in_port=7, ip, actions=ct(zone=64000, table=1)",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=100, table=1, ct_state=+trk+est, actions=output:5",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=100, table=1, ct_state=+trk+rel, actions=output:5",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=10, table=1, dl_dst=" + eth0MAC + ", actions=output:LOCAL",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=0, table=1, actions=output:NORMAL",
			"ovs-ofctl add-flow breth0 cookie=0xdeff105, priority=0, table=2, actions=output:7",
		})
		// nodePortWatcher()
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get interface patch-breth0_" + nodeName + "-to-br-int ofport",
			Output: "5",
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get interface eth0 ofport",
			Output: "7",
		})
		// syncServices()
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd: "ovs-ofctl dump-flows breth0",
			Output: `cookie=0x0, duration=8366.605s, table=0, n_packets=0, n_bytes=0, priority=100,ip,in_port="patch-breth0_no" actions=ct(commit,zone=64000),output:eth0
cookie=0x0, duration=8366.603s, table=0, n_packets=10642, n_bytes=10370438, priority=50,ip,in_port=eth0 actions=ct(table=1,zone=64000)
cookie=0x0, duration=8366.705s, table=0, n_packets=11549, n_bytes=1746901, priority=0 actions=FLOOD
cookie=0x0, duration=8366.602s, table=1, n_packets=0, n_bytes=0, priority=100,ct_state=+est+trk actions=output:"patch-breth0_no"
cookie=0x0, duration=8366.600s, table=1, n_packets=0, n_bytes=0, priority=100,ct_state=+rel+trk actions=output:"patch-breth0_no"
cookie=0x0, duration=8366.597s, table=1, n_packets=10641, n_bytes=10370087, priority=0 actions=LOCAL
`,
		})

		err := util.SetExec(fexec)
		Expect(err).NotTo(HaveOccurred())

		_, err = config.InitConfig(ctx, fexec, nil)
		Expect(err).NotTo(HaveOccurred())

		existingNode := v1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		}}

		kubeFakeClient := fake.NewSimpleClientset(&v1.NodeList{
			Items: []v1.Node{existingNode},
		})
		egressFirewallFakeClient := &egressfirewallfake.Clientset{}
		crdFakeClient := &apiextensionsfake.Clientset{}
		egressIPFakeClient := &egressipfake.Clientset{}
		fakeClient := &util.OVNClientset{
			KubeClient:           kubeFakeClient,
			EgressIPClient:       egressIPFakeClient,
			EgressFirewallClient: egressFirewallFakeClient,
			APIExtensionsClient:  crdFakeClient,
		}

		stop := make(chan struct{})
		wf, err := factory.NewNodeWatchFactory(fakeClient, nodeName)

		Expect(err).NotTo(HaveOccurred())
		defer func() {
			close(stop)
			wf.Shutdown()
		}()

		k := &kube.Kube{fakeClient.KubeClient, egressIPFakeClient, egressFirewallFakeClient}

		iptV4, iptV6 := util.SetFakeIPTablesHelpers()

		nodeAnnotator := kube.NewNodeAnnotator(k, &existingNode)

		err = util.SetNodeHostSubnetAnnotation(nodeAnnotator, ovntest.MustParseIPNets(nodeSubnet))
		Expect(err).NotTo(HaveOccurred())
		err = nodeAnnotator.Run()
		Expect(err).NotTo(HaveOccurred())

		err = testNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			gatewayNextHops, gatewayIntf, err := getGatewayNextHops()
			sharedGw, err := newSharedGateway(nodeName, ovntest.MustParseIPNets(nodeSubnet), gatewayNextHops, gatewayIntf, nodeAnnotator)
			Expect(err).NotTo(HaveOccurred())
			err = sharedGw.Init()
			Expect(err).NotTo(HaveOccurred())
			startGateway(sharedGw, wf)
			// check if IP addresses have been assigned to localnetGatewayNextHopPort interface
			link, err := netlink.LinkByName(localnetGatewayNextHopPort)
			Expect(err).NotTo(HaveOccurred())
			addresses, err := netlink.AddrList(link, syscall.AF_INET)
			Expect(err).NotTo(HaveOccurred())
			var foundAddr bool
			expectedAddress, err := netlink.ParseAddr(brNextHopCIDR)
			Expect(err).NotTo(HaveOccurred())
			for _, a := range addresses {
				if a.IP.Equal(expectedAddress.IP) && bytes.Equal(a.Mask, expectedAddress.Mask) {
					foundAddr = true
					break
				}
			}
			Expect(foundAddr).To(BeTrue())

			err = nodeAnnotator.Run()
			Expect(err).NotTo(HaveOccurred())

			go sharedGw.Run(stop)

			// Verify the code moved eth0's IP address, MAC, and routes
			// over to breth0
			l, err := netlink.LinkByName("breth0")
			Expect(err).NotTo(HaveOccurred())
			addrs, err := netlink.AddrList(l, syscall.AF_INET)
			Expect(err).NotTo(HaveOccurred())
			var found bool
			expectedAddr, err := netlink.ParseAddr(eth0CIDR)
			Expect(err).NotTo(HaveOccurred())
			for _, a := range addrs {
				if a.IP.Equal(expectedAddr.IP) && bytes.Equal(a.Mask, expectedAddr.Mask) {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())

			Expect(l.Attrs().HardwareAddr.String()).To(Equal(eth0MAC))
			return nil
		})

		Eventually(fexec.CalledMatchesExpected, 5).Should(BeTrue(), fexec.ErrorDesc)

		expectedTables := map[string]util.FakeTable{
			"filter": {
				"OUTPUT": []string{
					"-j OVN-KUBE-EXTERNALIP",
					"-j OVN-KUBE-NODEPORT",
				},
				"FORWARD": []string{
					"-j OVN-KUBE-EXTERNALIP",
					"-j OVN-KUBE-NODEPORT",
				},
				"OVN-KUBE-NODEPORT":   []string{},
				"OVN-KUBE-EXTERNALIP": []string{},
			},
			"nat": {
				"OUTPUT": []string{
					"-j OVN-KUBE-EXTERNALIP",
					"-j OVN-KUBE-NODEPORT",
				},
				"PREROUTING": []string{
					"-j OVN-KUBE-EXTERNALIP",
					"-j OVN-KUBE-NODEPORT",
				},
				"OVN-KUBE-NODEPORT":   []string{},
				"OVN-KUBE-EXTERNALIP": []string{},
			},
		}
		f4 := iptV4.(*util.FakeIPTables)
		err = f4.MatchState(expectedTables)
		Expect(err).NotTo(HaveOccurred())

		expectedTables = map[string]util.FakeTable{
			"filter": {},
			"nat":    {},
		}
		f6 := iptV6.(*util.FakeIPTables)
		err = f6.MatchState(expectedTables)
		Expect(err).NotTo(HaveOccurred())
		return nil
	}

	err := app.Run([]string{
		app.Name,
		"--cluster-subnets=" + clusterCIDR,
		"--init-gateways",
		"--gateway-interface=" + eth0Name,
		"--nodeport",
		"--gateway-vlanid=" + fmt.Sprintf("%d", gatewayVLANID),
		"--mtu=" + mtu,
	})
	Expect(err).NotTo(HaveOccurred())
}

/* FIXME with updated local gw mode
func localNetInterfaceTest(app *cli.App, testNS ns.NetNS,
	subnets []*net.IPNet, brNextHopCIDRs []*netlink.Addr, ipts []*util.FakeIPTables,
	expectedIPTablesRules []map[string]util.FakeTable) {

	const mtu string = "1234"

	app.Action = func(ctx *cli.Context) error {
		const (
			nodeName      string = "node1"
			brLocalnetMAC string = "11:22:33:44:55:66"
			systemID      string = "cb9ec8fa-b409-4ef3-9f42-d9283c47aac6"
		)

		fexec := ovntest.NewLooseCompareFakeExec()
		fakeOvnNode := NewFakeOVNNode(fexec)

		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-vsctl --timeout=15 --may-exist add-br br-local",
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get interface br-local mac_in_use",
			Output: brLocalnetMAC,
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-vsctl --timeout=15 set bridge br-local other-config:hwaddr=" + brLocalnetMAC,
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get Open_vSwitch . external_ids:ovn-bridge-mappings",
			Output: "",
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ovs-vsctl --timeout=15 set Open_vSwitch . external_ids:ovn-bridge-mappings=" + types.PhysicalNetworkName + ":br-local",
			"ovs-vsctl --timeout=15 --if-exists del-port br-local " + legacyLocalnetGatewayNextHopPort +
				" -- --may-exist add-port br-local " + localnetGatewayNextHopPort + " -- set interface " + localnetGatewayNextHopPort + " type=internal mtu_request=" + mtu + " mac=00\\:00\\:a9\\:fe\\:21\\:01",
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd:    "ovs-vsctl --timeout=15 --if-exists get Open_vSwitch . external_ids:system-id",
			Output: systemID,
		})
		fexec.AddFakeCmd(&ovntest.ExpectedCmd{
			Cmd: "ip rule",
			Output: "0:	from all lookup local\n32766:	from all lookup main\n32767:	from all lookup default\n",
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ip rule add from all table " + localnetGatewayExternalIDTable,
		})
		fexec.AddFakeCmdsNoOutputNoError([]string{
			"ip route list table " + localnetGatewayExternalIDTable,
		})

		existingNode := v1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		}}

		fakeOvnNode.start(ctx,
			&v1.NodeList{
				Items: []v1.Node{
					existingNode,
				},
			},
		)

		nodeAnnotator := kube.NewNodeAnnotator(&kube.Kube{fakeOvnNode.fakeClient.KubeClient, &egressipfake.Clientset{}, &egressfirewallfake.Clientset{}}, &existingNode)
		err := util.SetNodeHostSubnetAnnotation(nodeAnnotator, subnets)
		Expect(err).NotTo(HaveOccurred())
		err = nodeAnnotator.Run()
		Expect(err).NotTo(HaveOccurred())

		err = testNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			config.IPv4Mode = false
			config.IPv6Mode = false
			for _, subnet := range subnets {
				if utilnet.IsIPv6CIDR(subnet) {
					config.IPv6Mode = true
				} else {
					config.IPv4Mode = true
				}
			}

			gw, err := newLocalGateway(fakeOvnNode.watcher, fakeNodeName, subnets, nodeAnnotator, fakeOvnNode.recorder, primaryLinkName)
			Expect(err).NotTo(HaveOccurred())
			startGateway(gw, fakeOvnNode.watcher)

			// Check if IP has been assigned to LocalnetGatewayNextHopPort
			link, err := netlink.LinkByName(localnetGatewayNextHopPort)
			Expect(err).NotTo(HaveOccurred())
			addrs, err := netlink.AddrList(link, syscall.AF_UNSPEC)
			Expect(err).NotTo(HaveOccurred())

			var foundAddr bool
			for _, expectedAddr := range brNextHopCIDRs {
				foundAddr = false
				for _, a := range addrs {
					if a.IP.Equal(expectedAddr.IP) && bytes.Equal(a.Mask, expectedAddr.Mask) {
						foundAddr = true
						break
					}
				}
				Expect(foundAddr).To(BeTrue())
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(fexec.CalledMatchesExpected, 2).Should(BeTrue(), fexec.ErrorDesc)

		for i := 0; i < len(ipts); i++ {
			err = ipts[i].MatchState(expectedIPTablesRules[i])
			Expect(err).NotTo(HaveOccurred())
		}
		return nil
	}

	err := app.Run([]string{
		app.Name,
		"--init-gateways",
		"--gateway-local",
		"--nodeport",
		"--mtu=" + mtu,
	})
	Expect(err).NotTo(HaveOccurred())
}
*/

func expectedIPTablesRules(gatewayIP string) map[string]util.FakeTable {
	table := map[string]util.FakeTable{
		"filter": {
			"INPUT": []string{
				"-i " + localnetGatewayNextHopPort + " -m comment --comment from OVN to localhost -j ACCEPT",
			},
			"FORWARD": []string{
				"-j OVN-KUBE-EXTERNALIP",
				"-j OVN-KUBE-NODEPORT",
				"-o " + localnetGatewayNextHopPort + " -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
				"-i " + localnetGatewayNextHopPort + " -j ACCEPT",
			},
			"OVN-KUBE-NODEPORT":   []string{},
			"OVN-KUBE-EXTERNALIP": []string{},
		},
		"nat": {
			"POSTROUTING": []string{
				"-s " + gatewayIP + " -j MASQUERADE",
			},
			"PREROUTING": []string{
				"-j OVN-KUBE-EXTERNALIP",
				"-j OVN-KUBE-NODEPORT",
			},
			"OUTPUT": []string{
				"-j OVN-KUBE-EXTERNALIP",
				"-j OVN-KUBE-NODEPORT",
			},
			"OVN-KUBE-NODEPORT":   []string{},
			"OVN-KUBE-EXTERNALIP": []string{},
		},
	}

	// OCP HACK: Block MCS Access. https://github.com/openshift/ovn-kubernetes/pull/170
	table["filter"]["FORWARD"] = append(table["filter"]["FORWARD"],
		"-p tcp -m tcp --dport 22624 -j REJECT",
		"-p tcp -m tcp --dport 22623 -j REJECT",
	)
	table["filter"]["OUTPUT"] = append(table["filter"]["OUTPUT"],
		"-p tcp -m tcp --dport 22624 -j REJECT",
		"-p tcp -m tcp --dport 22623 -j REJECT",
	)
	// END OCP HACK

	return table
}

var _ = Describe("Gateway Init Operations", func() {

	var (
		testNS ns.NetNS
		app    *cli.App
	)

	BeforeEach(func() {
		var err error
		testNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())

		// Restore global default values before each testcase
		config.PrepareTestConfig()

		app = cli.NewApp()
		app.Name = "test"
		app.Flags = config.Flags

		// Set up a fake br-local & LocalnetGatewayNextHopPort
		testNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		err = testNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()

			ovntest.AddLink("br-local")
			ovntest.AddLink(localnetGatewayNextHopPort)

			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(testNS.Close()).To(Succeed())
	})
	/* FIXME for updated local gw mode
	Context("for localnet operations", func() {
		const (
			v4BrNextHopIP       = "169.254.33.1"
			v4BrNextHopCIDR     = v4BrNextHopIP + "/24"
			v4NodeSubnet        = "10.1.1.0/24"
			v4localnetGatewayIP = "169.254.33.2"

			v6BrNextHopIP       = "fd99::1"
			v6BrNextHopCIDR     = v6BrNextHopIP + "/64"
			v6NodeSubnet        = "2001:db8:abcd:0012::0/64"
			v6localnetGatewayIP = "fd99::2"
		)
		var (
			brNextHopCIDRs []*netlink.Addr
			ipts           []*util.FakeIPTables
			ipTablesRules  []map[string]util.FakeTable
		)

		It("sets up a IPv4 localnet gateway", func() {
			nextHopCIDRIPv4, err := netlink.ParseAddr(v4BrNextHopCIDR)
			Expect(err).NotTo(HaveOccurred())
			brNextHopCIDRs := append(brNextHopCIDRs, nextHopCIDRIPv4)

			v4ipt, _ := util.SetFakeIPTablesHelpers()
			ipts := append(ipts, v4ipt.(*util.FakeIPTables))
			ipTablesRules := append(ipTablesRules, expectedIPTablesRules(v4localnetGatewayIP))

			localNetInterfaceTest(app, testNS, ovntest.MustParseIPNets(v4NodeSubnet), brNextHopCIDRs, ipts, ipTablesRules)
		})

		It("sets up a IPv6 localnet gateway", func() {
			nextHopCIDRIPv6, err := netlink.ParseAddr(v6BrNextHopCIDR)
			Expect(err).NotTo(HaveOccurred())
			brNextHopCIDRs := append(brNextHopCIDRs, nextHopCIDRIPv6)

			_, v6ipt := util.SetFakeIPTablesHelpers()
			ipts := append(ipts, v6ipt.(*util.FakeIPTables))
			ipTablesRules := append(ipTablesRules, expectedIPTablesRules(v6localnetGatewayIP))

			localNetInterfaceTest(app, testNS, ovntest.MustParseIPNets(v6NodeSubnet), brNextHopCIDRs, ipts, ipTablesRules)
		})

		It("sets up a dual stack localnet gateway", func() {
			nextHopCIDRIPv4, err := netlink.ParseAddr(v4BrNextHopCIDR)
			Expect(err).NotTo(HaveOccurred())
			brNextHopCIDRs := append(brNextHopCIDRs, nextHopCIDRIPv4)
			nextHopCIDRIPv6, err := netlink.ParseAddr(v6BrNextHopCIDR)
			Expect(err).NotTo(HaveOccurred())
			brNextHopCIDRs = append(brNextHopCIDRs, nextHopCIDRIPv6)

			v4ipt, v6ipt := util.SetFakeIPTablesHelpers()
			ipts := append(ipts, v4ipt.(*util.FakeIPTables))
			ipts = append(ipts, v6ipt.(*util.FakeIPTables))
			ipTablesRules := append(ipTablesRules, expectedIPTablesRules(v4localnetGatewayIP))
			ipTablesRules = append(ipTablesRules, expectedIPTablesRules(v6localnetGatewayIP))

			localNetInterfaceTest(app, testNS, ovntest.MustParseIPNets(v4NodeSubnet, v6NodeSubnet), brNextHopCIDRs,
				ipts, ipTablesRules)
		})
	})
	*/

	Context("for NIC-based operations", func() {
		const (
			eth0Name string = "eth0"
			eth0IP   string = "192.168.1.10"
			eth0CIDR string = eth0IP + "/24"
			eth0GWIP string = "192.168.1.1"
		)
		var eth0MAC string

		BeforeEach(func() {
			// Set up a fake eth0
			err := testNS.Do(func(ns.NetNS) error {
				defer GinkgoRecover()

				ovntest.AddLink(eth0Name)

				l, err := netlink.LinkByName(eth0Name)
				Expect(err).NotTo(HaveOccurred())
				err = netlink.LinkSetUp(l)
				Expect(err).NotTo(HaveOccurred())

				// Add an IP address
				addr, err := netlink.ParseAddr(eth0CIDR)
				Expect(err).NotTo(HaveOccurred())
				err = netlink.AddrAdd(l, addr)
				Expect(err).NotTo(HaveOccurred())

				eth0MAC = l.Attrs().HardwareAddr.String()

				// And a default route
				err = netlink.RouteAdd(&netlink.Route{
					LinkIndex: l.Attrs().Index,
					Scope:     netlink.SCOPE_UNIVERSE,
					Dst:       ovntest.MustParseIPNet("0.0.0.0/0"),
					Gw:        ovntest.MustParseIP(eth0GWIP),
				})
				Expect(err).NotTo(HaveOccurred())

				return nil
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("sets up a shared interface gateway", func() {
			shareGatewayInterfaceTest(app, testNS, eth0Name, eth0MAC, eth0IP, eth0GWIP, eth0CIDR, 0)
		})

		It("sets up a shared interface gateway with tagged VLAN", func() {
			shareGatewayInterfaceTest(app, testNS, eth0Name, eth0MAC, eth0IP, eth0GWIP, eth0CIDR, 3000)
		})

	})
})
