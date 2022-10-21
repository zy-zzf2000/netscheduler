package plugin

import (
	"context"
	"math"
	"strconv"

	"netbalance/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

//实现PreFilterPlugin，ScorePlugin接口
var (
	_ framework.PreFilterPlugin = &BalanceNetScheduling{}
	_ framework.ScorePlugin     = &BalanceNetScheduling{}
)

//自定义调度器名字
const Name = "ouo-scheduler"

//BWNA调度器插件结构体
type BalanceNetScheduling struct {
	handle              framework.Handle   //调度器句柄,用于访问kube-apiserver
	resourceToWeightMap map[string]float64 //存放CPU、内存与网络权重的map
}

type NetResourceMap struct {
	mmap map[string]float64
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
	plugin.resourceToWeightMap = map[string]float64{
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
		capacityMap.mmap = make(map[string]float64)
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
	var Cnet, Cmemory, Ccpu float64
	var Rnet, Rmemory, Rcpu float64
	var Tnet, Tmemory, Tcpu float64
	var Unet, Umemory, Ucpu float64
	//获取节点CPU、内存、网络资源的Capacity
	node, err := n.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, err.Error())
	}

	capacityMap, err := state.Read("NodeNetCapacityMap")
	if err != nil {
		return 0, framework.NewStatus(framework.Error, err.Error())
	}
	Cnet = capacityMap.(*NetResourceMap).mmap[nodeName]
	Ccpu = node.Node().Status.Capacity.Cpu().AsApproximateFloat64()
	Cmemory = node.Node().Status.Capacity.Memory().AsApproximateFloat64()

	//获取节点CPU、内存、网络资源的已经被使用的数目
	cpuAllocatable := node.Node().Status.Allocatable.Cpu().AsApproximateFloat64()
	memoryAllocatable := node.Node().Status.Allocatable.Memory().AsApproximateFloat64()

	Ucpu = Ccpu - cpuAllocatable
	Umemory = Cmemory - memoryAllocatable
	Unet = util.QueryNetUsageByNode(nodeName)

	//获取当前pod的CPU、内存、网络资源的申请数目
	containerNum := len(p.Spec.Containers)
	Rcpu = 0
	Rmemory = 0
	Rnet = 0
	for i := 0; i < containerNum; i++ {
		Rcpu += float64(p.Spec.Containers[i].Resources.Requests.Cpu().Value())
		Rmemory += float64(p.Spec.Containers[i].Resources.Requests.Memory().Value())
		net, err := strconv.ParseFloat((p.Labels["netRequest"]), 64)
		if err != nil {
			return 0, framework.NewStatus(framework.Error, err.Error())
		}
		Rnet += net
	}

	//计算节点运行该Pod后的CPU、内存、网络资源的使用量
	Tcpu = Ucpu + Rcpu
	Tmemory = Umemory + Rmemory
	Tnet = Unet + Rnet

	//计算节点运行该Pod后的CPU、内存、网络资源的剩余量
	ECpu := Ccpu - Tcpu
	Ememory := Cmemory - Tmemory
	Enet := Cnet - Tnet

	//带入公式计算得分
	scorePart1 := (1 / (n.resourceToWeightMap["cpu"] + n.resourceToWeightMap["memory"] + n.resourceToWeightMap["net"])) *
		((ECpu*n.resourceToWeightMap["cpu"]/Ccpu + Ememory*n.resourceToWeightMap["memory"]/Cmemory) + Enet*float64(n.resourceToWeightMap["net"])/Cnet)
	scorePart2 := math.Abs(Ucpu/Ccpu-Umemory/Cmemory) + math.Abs(Ucpu/Ccpu-Unet/Cnet) + math.Abs(Unet/Cnet-Umemory/Cmemory)
	finalScore := scorePart1 - scorePart2/3

	return int64(finalScore), framework.NewStatus(framework.Success, "")
}

func (n *BalanceNetScheduling) ScoreExtensions() framework.ScoreExtensions {
	return nil
}
