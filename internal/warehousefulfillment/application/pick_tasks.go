package application

import "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"

// countedTasks 只留算这张出库请求已拣的任务：拣满与少拣。
// 待执行与作废的没有拣出货。迟到的是任务作废之后才拣出来的，货记在它自己的回库任务上，要回库，不算这张请求的已拣。
// 放出后撤单让还在往下走的请求也会带着作废与迟到的任务：复核与打包要对的数量、截单与拆包要回库的货、
// 出库确认消耗的待出库库位，都只认这里留下的。把迟到的也算进去，复核数量永远对不上，截单会给同一条任务排第二条回库。
func countedTasks(tasks []ports.PickTask) []ports.PickTask {
	counted := make([]ports.PickTask, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == "DONE" || task.Status == "SHORT" {
			counted = append(counted, task)
		}
	}
	return counted
}

// pickedQuantity 是这张出库请求已拣的数量，口径同 countedTasks。
func pickedQuantity(tasks []ports.PickTask) int64 {
	var picked int64
	for _, task := range countedTasks(tasks) {
		picked += task.Picked
	}
	return picked
}
