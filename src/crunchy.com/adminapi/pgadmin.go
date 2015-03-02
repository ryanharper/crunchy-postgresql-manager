/*
 Copyright 2015 Crunchy Data Solutions, Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package main

import (
	"crunchy.com/admindb"
	"crunchy.com/cpmagent"
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/golang/glog"
	"net/http"
	"time"
)

func AdminStartpg(w rest.ResponseWriter, r *rest.Request) {
	err := secimpl.Authorize(r.PathParam("Token"), "perm-cluster")
	if err != nil {
		glog.Errorln("AdminStartpg: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	ID := r.PathParam("ID")
	if ID == "" {
		glog.Errorln("AdminStartpg: error node ID required")
		rest.Error(w, "node ID required", http.StatusBadRequest)
		return
	}

	dbNode, err := admindb.GetDBNode(ID)
	if err != nil {
		glog.Errorln("AdminStartpg: " + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var output string
	output, err = cpmagent.AgentCommand(CPMBIN+"startpg.sh", "", dbNode.Name)
	if err != nil {
		glog.Errorln("AdminStartpg:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.Infoln("AdminStartpg:" + output)

	//give the UI a chance to see the start
	time.Sleep(3000 * time.Millisecond)

	w.WriteHeader(http.StatusOK)
	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteJson(&status)
}

func AdminStoppg(w rest.ResponseWriter, r *rest.Request) {

	err := secimpl.Authorize(r.PathParam("Token"), "perm-cluster")
	if err != nil {
		glog.Errorln("AdminStoppg: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	glog.Infoln("AdminStoppg:called")
	ID := r.PathParam("ID")
	if ID == "" {
		glog.Errorln("AdminStoppg:ID not found error")
		rest.Error(w, "node ID required", http.StatusBadRequest)
		return
	}

	dbNode, err := admindb.GetDBNode(ID)
	if err != nil {
		glog.Errorln("AdminStartpg: " + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.Infoln("AdminStoppg: in stop with dbnode")

	var output string
	output, err = cpmagent.AgentCommand(CPMBIN+"stoppg.sh", "", dbNode.Name)
	if err != nil {
		glog.Errorln("AdminStoppg:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	glog.Infoln("AdminStoppg:" + output)

	//give the UI a chance to see the stop
	time.Sleep(3000 * time.Millisecond)

	w.WriteHeader(http.StatusOK)
	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteJson(&status)
}