// Copyright 2017 XUEQIU.COM
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

package dump

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
)

func index(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	for key := range counters.Items() {
		http.Redirect(w, r, "/instance/"+key.(string), http.StatusFound)
		return
	}
}

func rdbReveal(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// deep copy  tplCommonData into data
	data := map[string]interface{}{}
	for key, val := range tplCommonData {
		data[key] = val
	}

	path := p.ByName("path")

	c := counters.Get(path)
	if c == nil {
		return
	}
	counter := c.(*Counter)

	topN, _ := strconv.Atoi(data["TopN"].(string))
	sizeFilter, _ := strconv.ParseInt(data["sizeFilter"].(string), 10, 64)

	data["CurrentInstance"] = path
	data["TopBigKeysByCountByte"] = counter.GetLargestEntries(topN, sizeFilter) //top N bigkey (按内存),第2个参数过滤掉小于阈值的key，传0表示不过滤
	data["TopBigKeysByCountNum"] = counter.GetLargestEntriesByCount(topN) //top N bigkey (按元素个数)

	largestKeyPrefixesByType := map[string][]*PrefixEntry{}
	for _, entry := range counter.GetLargestKeyPrefixes() {
		// mem use less than 1M, and list is long enough, not necessary to add
		if entry.Bytes < 1000*1000 && len(largestKeyPrefixesByType[entry.Type]) > 50 {
			continue
		}
		largestKeyPrefixesByType[entry.Type] = append(largestKeyPrefixesByType[entry.Type], entry)
	}
	data["LargestKeyPrefixes"] = largestKeyPrefixesByType

	data["TypeBytes"] = counter.typeBytes
	data["TypeNum"] = counter.typeNum
	totalNum := uint64(0)
	for _, v := range counter.typeNum {
		totalNum += v
	}
	totalBytes := uint64(0)
	for _, v := range counter.typeBytes {
		totalBytes += v
	}
	data["TotalNum"] = totalNum
	data["TotalBytes"] = totalBytes

	lenLevelCount := map[string][]*PrefixEntry{}
	for _, entry := range counter.GetLenLevelCount() {
		lenLevelCount[entry.Type] = append(lenLevelCount[entry.Type], entry)
	}
	data["LenLevelCount"] = lenLevelCount


	data["ExpireStatBytes"] = counter.expireStatBytes
	data["ExpireStatNum"] = counter.expireStatNum
	data["ExpireStatOrder"] = counter.expireStatOrder

	data["ByteLevelNum"] = counter.byteLevelNum
	data["ByteLevelBytes"] = counter.byteLevelBytes
	data["ByteLevelOrder"] = counter.byteLevelOrder

	ServeHTML(w, "base.html", "revel.html", data)
}
