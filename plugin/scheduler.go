package plugin

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

//实现PreFilterPlugin，ScorePlugin接口
var (
	_ framework.PreFilterPlugin = &BalanceNetScheduling{}
	_ framework.ScorePlugin     = &BalanceNetScheduling{}
)

//自定义调度器名字
const Name = "BalanceNetscheduler"

//BWNA调度器插件结构体
type BalanceNetScheduling struct {
	handle              framework.Handle //调度器句柄,用于访问kube-apiserver
	resourceToWeightMap map[string]int64 //存放CPU、内存与网络权重的map
}

type NetResourceMap struct {
	mmap map[string]int64
}

//根据文档，Clone方法需要实现浅拷贝
func (m *NetResourceMap) Clone() framework.StateData {
	c := &NetResourceMap{
		mmap: m.mmap,
	}
	return c
}

func (n *BalanceNetScheduling) Name() string {
	return Name
}

//用于初始化调度器插件
func New(configuration runtime.Object, f framework.Handle) (framework.Plugin, error) {
	//TODO:后续实现从配置文件中读取权重
	plugin := &BalanceNetScheduling{}
	plugin.handle = f
	plugin.resourceToWeightMap = map[string]int64{
		"cpu":    1,
		"memory": 1,
		"net":    1,
	}
	return plugin, nil
}

func (n *BalanceNetScheduling) PreFilter(ctx context.Context, state *framework.CycleState, p *v1.Pod) *framework.Status {
	/*
		TODO:初始化集群中所有节点的网络资源的Request(已经申请的网络资源)和Capacity(节点的网络资源总量)
		在一个新的集群中，所有节点的Request都为0，Capacity为节点的网络资源总量
		分别使用两个Map对象NodeNetRequestMap和NodeNetCapacityMap存储所有节点的网络Request和Capacity
		然后将这两个Map对象存储到CycleState中
	*/

	//获取所有节点的名称，初始化每个Node的Request和Capacity
	nodeList, err := n.handle.SnapshotSharedLister().NodeInfos().List()
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	/*
		首先尝试从CycleState中获取NodeNetRequestMap和NodeNetCapacityMap
		如果获取失败，则说明是一个新的集群，需要初始化这两个Map对象
		如果获取成功，则说明不是一个新的集群，不需要初始化这两个Map对象
	*/
	//FIXME:写入CycleState中的数据需要实现Clone方法，因此需要将map封装成一个结构体，然后实现Clone方法(Done)
	//FIXME:获取网络资源的Capacity，这里暂定为100m
	//FIXME:获取已经request的资源用prometheus来实现，因此不需要requestMap
	_, err = state.Read("NodeNetCapacityMap")

	if err != nil {
		capacityMap := NetResourceMap{}
		capacityMap.mmap = make(map[string]int64)
		//初始化NodeNetCapacityMap
		for _, node := range nodeList {
			//capacityMap.mmap[node.Node().Name] = node.Node().Status.Capacity.;
			capacityMap.mmap[node.Node().Name] = 50
		}
		//将NodeNetCapacityMap存储到CycleState中
		state.Write("NodeNetCapacityMap", &capacityMap)
	}

	return framework.NewStatus(framework.Success, "")
}

func (n *BalanceNetScheduling) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (n *BalanceNetScheduling) Score(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) (int64, *framework.Status) {
	/*
		基本思路是：
		1.直接利用handler获取节点的CPU、内存信息
		2.网络资源的Capacity从CycleState中获取，而已经申请的网络资源从Promehteus中获取
		3.计算节点运行该Pod后的CPU、内存、网络资源的剩余量
		4.带入公式计算得分
	*/
	//获取节点CPU、内存、网络资源的Capacity
	node, err := n.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, err.Error())
	}
	cpuCapacity := node.Node().Status.Capacity.Cpu().Value()
	memoryCapacity := node.Node().Status.Capacity.Memory().Value()
	capacityMap, err := state.Read("NodeNetCapacityMap")
	if err != nil {
		return 0, framework.NewStatus(framework.Error, err.Error())
	}
	netCapacity := capacityMap.(*NetResourceMap).mmap[nodeName]

	//获取节点CPU、内存、网络资源的已经申请的数目
	cpuAllocatable := node.Node().Status.Allocatable.Cpu().Value()
	cpuUsed := cpuCapacity - cpuAllocatable
	memoryAllocatable := node.Node().Status.Allocatable.Memory().Value()
	memoryUsed := memoryCapacity - memoryAllocatable
	//query: irate(node_network_transmit_bytes_total{instance="116.56.140.131:9100", device!~"lo|bond[0-9]|cbr[0-9]|veth.*"}[5m]) > 0 ,单位为bytes/sec

	return 0, nil
}

func (n *BalanceNetScheduling) ScoreExtensions() framework.ScoreExtensions {
	return nil
}
