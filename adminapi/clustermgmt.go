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

package adminapi

import (
	"errors"
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/crunchydata/crunchy-postgresql-manager/admindb"
	"github.com/crunchydata/crunchy-postgresql-manager/cpmagent"
	"github.com/crunchydata/crunchy-postgresql-manager/kubeclient"
	"github.com/crunchydata/crunchy-postgresql-manager/logit"
	"github.com/crunchydata/crunchy-postgresql-manager/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type AutoClusterInfo struct {
	Name           string
	ClusterType    string
	ClusterProfile string
	Token          string
}

func GetCluster(w rest.ResponseWriter, r *rest.Request) {
	err := secimpl.Authorize(r.PathParam("Token"), "perm-read")
	if err != nil {
		logit.Error.Println("GetCluster: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	ID := r.PathParam("ID")
	results, err := admindb.GetDBCluster(ID)
	if err != nil {
		logit.Error.Println("GetCluster:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
	}
	cluster := Cluster{results.ID, results.Name, results.ClusterType,
		results.Status, results.CreateDate, ""}
	logit.Info.Println("GetCluser:db call results=" + results.ID)

	w.WriteJson(&cluster)
}

func ConfigureCluster(w rest.ResponseWriter, r *rest.Request) {

	err := secimpl.Authorize(r.PathParam("Token"), "perm-cluster")
	if err != nil {
		logit.Error.Println("ConfigureCluster: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	ID := r.PathParam("ID")
	cluster, err := admindb.GetDBCluster(ID)
	if err != nil {
		logit.Error.Println("ConfigureCluster:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = configureCluster(cluster, false)
	if err != nil {
		logit.Error.Println("ConfigureCluster:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteJson(&status)
}

func configureCluster(cluster admindb.DBCluster, autocluster bool) error {
	logit.Info.Println("configureCluster:GetDBCluster")

	//get master node for this cluster
	master, err := admindb.GetDBNodeMaster(cluster.ID)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:GetDBNodeMaster")

	//configure master postgresql.conf file
	var data string
	if cluster.ClusterType == "synchronous" {
		data, err = template.Postgresql("master", "5432", "*")
	} else {
		data, err = template.Postgresql("master", "5432", "")
	}
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:master postgresql.conf generated")

	//write master postgresql.conf file remotely
	err = RemoteWritefile("/pgdata/postgresql.conf", data, master.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:master postgresql.conf copied to remote")

	//get domain name
	var domainname admindb.DBSetting
	domainname, err = admindb.GetDBSetting("DOMAIN-NAME")
	if err != nil {
		logit.Error.Println("configureCluster: DOMAIN-NAME err " + err.Error())
		return err
	}

	//configure master pg_hba.conf file
	data, err = template.Hba(KubeEnv, "master", master.Name, "5432", cluster.ID, domainname.Value)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:master pg_hba.conf generated")

	//write master pg_hba.conf file remotely
	err = RemoteWritefile("/pgdata/pg_hba.conf", data, master.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:master pg_hba.conf copied remotely")

	//restart postgres after the config file changes
	var commandoutput string
	commandoutput, err = PGCommand(CPMBIN+"stoppg.sh", master.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	logit.Info.Println("configureCluster: master stoppg output was" + commandoutput)

	commandoutput, err = PGCommand(CPMBIN+"startpg.sh", master.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	logit.Info.Println("configureCluster:master startpg output was" + commandoutput)

	//sleep loop until the master's PG can respond
	var found = false
	var currentStatus string
	var masterhost = master.Name
	if KubeEnv {
		masterhost = master.Name + "-db"
	}
	for i := 0; i < 20; i++ {
		currentStatus, err = GetPGStatus2(masterhost)
		if currentStatus == "RUNNING" {
			logit.Info.Println("master is running...continuing")
			found = true
			break
		} else {
			logit.Info.Println("sleeping 1 sec waiting on master..")
			time.Sleep(1000 * time.Millisecond)
		}
	}
	if !found {
		logit.Info.Println("configureCluster: timed out waiting on master pg to start")
		return errors.New("timeout waiting for master pg to respond")
	}

	standbynodes, err2 := admindb.GetAllDBStandbyNodes(cluster.ID)
	if err2 != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	//configure all standby nodes
	i := 0
	for i = range standbynodes {
		if standbynodes[i].Role == "standby" {

			//stop standby
			if !autocluster {
				commandoutput, err = PGCommand(CPMBIN+"stoppg.sh", standbynodes[i].Name)
				if err != nil {
					logit.Error.Println("configureCluster:" + err.Error())
					return err
				}
				logit.Info.Println("configureCluster:stop output was" + commandoutput)
			}

			//create base backup from master
			commandoutput, err = cpmagent.Command1(CPMBIN+"basebackup.sh", masterhost+"."+domainname.Value, standbynodes[i].Name)
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}
			logit.Info.Println("configureCluster:basebackup output was" + commandoutput)

			data, err = template.Recovery(masterhost, "5432", "postgres")
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}
			logit.Info.Println("configureCluster:standby recovery.conf generated")

			//write standby recovery.conf file remotely
			err = RemoteWritefile("/pgdata/recovery.conf", data, standbynodes[i].Name)
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}
			logit.Info.Println("configureCluster:standby recovery.conf copied remotely")

			data, err = template.Postgresql("standby", "5432", "")
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}

			//write standby postgresql.conf file remotely
			err = RemoteWritefile("/pgdata/postgresql.conf", data, standbynodes[i].Name)
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}
			logit.Info.Println("configureCluster:standby postgresql.conf copied remotely")

			//configure standby pg_hba.conf file
			data, err = template.Hba(KubeEnv, "standby", standbynodes[i].Name, "5432", cluster.ID, domainname.Value)
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}

			logit.Info.Println("configureCluster:standby pg_hba.conf generated")

			//write standby pg_hba.conf file remotely
			err = RemoteWritefile("/pgdata/pg_hba.conf", data, standbynodes[i].Name)
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}
			logit.Info.Println("configureCluster:standby pg_hba.conf copied remotely")

			//start standby

			commandoutput, err = PGCommand(CPMBIN+"startpgonstandby.sh", standbynodes[i].Name)
			if err != nil {
				logit.Error.Println("configureCluster:" + err.Error())
				return err
			}
			logit.Info.Println("configureCluster:standby startpg output was" + commandoutput)
		}
		i++
	}

	logit.Info.Println("configureCluster: sleeping 5 seconds before configuring pgpool...")
	time.Sleep(5000 * time.Millisecond)

	pgpoolNode, err4 := admindb.GetDBNodePgpool(cluster.ID)
	if err4 != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	logit.Info.Println("configureCluster:" + pgpoolNode.Name)

	//configure the pgpool includes all standby nodes AND the master node
	poolnames := make([]string, len(standbynodes)+1)

	i = 0
	for i = range standbynodes {
		poolnames[i] = standbynodes[i].Name + "." + domainname.Value
		i++
	}
	poolnames[i] = master.Name + "." + domainname.Value

	//generate pgpool.conf HOST_LIST
	data, err = template.Poolconf(poolnames)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:pgpool pgpool.conf generated")

	//write pgpool.conf to remote pool node
	err = RemoteWritefile(CPMBIN+"pgpool.conf", data, pgpoolNode.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	logit.Info.Println("configureCluster:pgpool pgpool.conf copied remotely")

	//generate pool_passwd
	data, err = template.Poolpasswd()
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:pgpool pool_passwd generated")

	//write pgpool.conf to remote pool node
	err = RemoteWritefile(CPMBIN+"pool_passwd", data, pgpoolNode.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	logit.Info.Println("configureCluster:pgpool pool_passwd copied remotely")

	//generate pool_hba.conf
	data, err = template.Poolhba()
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	logit.Info.Println("configureCluster:pgpool pool_hba generated")

	//write pgpool.conf to remote pool node
	err = RemoteWritefile(CPMBIN+"pool_hba.conf", data, pgpoolNode.Name)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}
	logit.Info.Println("configureCluster:pgpool pool_hba copied remotely")

	//start pgpool
	commandoutput, err = PGCommand(CPMBIN+"startpgpool.sh", pgpoolNode.Name)
	if err != nil {
		logit.Error.Println("configureCluster: " + err.Error())
		return err
	}
	logit.Info.Println("configureCluster: pgpool startpgpool output was" + commandoutput)

	//finally, update the cluster to show that it is
	//initialized!
	cluster.Status = "initialized"
	err = admindb.UpdateDBCluster(cluster)
	if err != nil {
		logit.Error.Println("configureCluster:" + err.Error())
		return err
	}

	return nil

}

func GetAllClusters(w rest.ResponseWriter, r *rest.Request) {
	err := secimpl.Authorize(r.PathParam("Token"), "perm-read")
	if err != nil {
		logit.Error.Println("GetAllClusters: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	results, err := admindb.GetAllDBClusters()
	if err != nil {
		logit.Error.Println("GetAllClusters: error-" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
	}
	clusters := make([]Cluster, len(results))
	i := 0
	for i = range results {
		clusters[i].ID = results[i].ID
		clusters[i].Name = results[i].Name
		clusters[i].ClusterType = results[i].ClusterType
		clusters[i].Status = results[i].Status
		clusters[i].CreateDate = results[i].CreateDate
		i++
	}

	w.WriteJson(&clusters)
}

//we use POST for both updating and inserting based on the ID passed in
func PostCluster(w rest.ResponseWriter, r *rest.Request) {
	logit.Info.Println("PostCluster: in PostCluster")
	cluster := Cluster{}
	err := r.DecodeJsonPayload(&cluster)
	if err != nil {
		logit.Error.Println("PostCluster: error in decode" + err.Error())
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = secimpl.Authorize(cluster.Token, "perm-cluster")
	if err != nil {
		logit.Error.Println("PostCluster: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if cluster.Name == "" {
		logit.Error.Println("PostCluster: error in Name")
		rest.Error(w, "cluster name required", http.StatusBadRequest)
		return
	}

	logit.Info.Println("PostCluster: have ID=" + cluster.ID + " Name=" + cluster.Name + " type=" + cluster.ClusterType + " status=" + cluster.Status)
	dbcluster := admindb.DBCluster{cluster.ID, cluster.Name, cluster.ClusterType, cluster.Status, ""}
	if cluster.ID == "" {
		strid, err := admindb.InsertDBCluster(dbcluster)
		newid := strconv.Itoa(strid)
		if err != nil {
			logit.Error.Println("PostCluster:" + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cluster.ID = newid
	} else {
		logit.Info.Println("PostCluster: about to call UpdateDBCluster")
		err2 := admindb.UpdateDBCluster(dbcluster)
		if err2 != nil {
			logit.Error.Println("PostCluster: error in UpdateDBCluster " + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.WriteJson(&cluster)
}

func DeleteCluster(w rest.ResponseWriter, r *rest.Request) {

	err := secimpl.Authorize(r.PathParam("Token"), "perm-cluster")
	if err != nil {
		logit.Error.Println("DeleteCluster: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	ID := r.PathParam("ID")
	if ID == "" {
		logit.Error.Println("DeleteCluster: error cluster ID required")
		rest.Error(w, "cluster ID required", http.StatusBadRequest)
		return
	}

	cluster, err := admindb.GetDBCluster(ID)
	if err != nil {
		logit.Error.Println("DeleteCluster:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
	}

	//delete docker containers
	containers, err := admindb.GetAllDBNodesForCluster(ID)
	if err != nil {
		logit.Error.Println("DeleteCluster:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
	}

	i := 0

	//handle the case where we want to delete a cluster but
	//it is not initialized, we can reuse the containers
	if cluster.Status == "uninitialized" {
		logit.Info.Println("DeleteCluster: delete cluster but not the nodes")
		for i = range containers {
			containers[i].ClusterID = "-1"
			err = admindb.UpdateDBNode(containers[i])
			if err != nil {
				logit.Error.Println("DeleteCluster:" + err.Error())
				rest.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		err = admindb.DeleteDBCluster(ID)
		if err != nil {
			logit.Error.Println("DeleteCluster:" + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		status := SimpleStatus{}
		status.Status = "OK"
		w.WriteHeader(http.StatusOK)
		w.WriteJson(&status)
		return
	}

	i = 0
	var output string
	server := admindb.DBServer{}
	for i = range containers {

		//go get the docker server IPAddress
		server, err = admindb.GetDBServer(containers[i].ServerID)
		if err != nil {
			logit.Error.Println("DeleteCluster:" + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		logit.Info.Println("DeleteCluster: got server IP " + server.IPAddress)

		//it is possible that someone can remove a container
		//outside of us, so we let it pass that we can't remove
		//it
		//err = removeContainer(server.IPAddress, containers[i].Name)
		if KubeEnv {
			//delete the kube pod with this name
			err = kubeclient.DeletePod(KubeURL, containers[i].Name)
			if err != nil {
				logit.Error.Println("DeleteCluster:" + err.Error())
				rest.Error(w, "error in deleting pod", http.StatusBadRequest)
				return
			}
			//delete the kube service with this name 13000
			err = kubeclient.DeleteService(KubeURL, containers[i].Name)
			if err != nil {
				logit.Error.Println("DeleteCluster:" + err.Error())
				rest.Error(w, "error in deleting service 1", http.StatusBadRequest)
				return
			}
			//delete the kube service with this name 5432
			err = kubeclient.DeleteService(KubeURL, containers[i].Name+"-db")
			if err != nil {
				logit.Error.Println("DeleteCluster:" + err.Error())
				rest.Error(w, "error in deleting service 1", http.StatusBadRequest)
				return
			}

		} else {
			output, err = cpmagent.DockerRemoveContainer(containers[i].Name,
				server.IPAddress)
			if err != nil {
				logit.Error.Println("DeleteCluster: error when trying to remove container" + err.Error())
			}
		}

		//send the server a deletevolume command
		output, err = cpmagent.AgentCommand(CPMBIN+"deletevolume", server.PGDataPath+"/"+containers[i].Name, server.IPAddress)
		logit.Info.Println("DeleteCluster:" + output)

		i++
	}

	//delete the container entries
	//delete the cluster entry
	admindb.DeleteDBCluster(ID)

	for i = range containers {

		err = admindb.DeleteDBNode(containers[i].ID)
		if err != nil {
			logit.Error.Println("DeleteCluster:" + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteHeader(http.StatusOK)
	w.WriteJson(&status)
}

func AdminFailover(w rest.ResponseWriter, r *rest.Request) {
	err := secimpl.Authorize(r.PathParam("Token"), "perm-cluster")
	if err != nil {
		logit.Error.Println("AdminFailover: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	ID := r.PathParam("ID")
	if ID == "" {
		logit.Error.Println("AdminFailover: node ID required error")
		rest.Error(w, "node ID required", http.StatusBadRequest)
		return
	}

	//dbNode is the standby node we are going to fail over and
	//make the new master in the cluster
	var dbNode admindb.DBClusterNode
	dbNode, err = admindb.GetDBNode(ID)
	if err != nil {
		logit.Error.Println("AdminFailover:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var output string

	cluster, err := admindb.GetDBCluster(dbNode.ClusterID)
	if err != nil {
		logit.Error.Println("AdminFailover:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	output, err = cpmagent.AgentCommand(CPMBIN+"fail-over.sh", dbNode.Name, dbNode.Name)
	if err != nil {
		logit.Error.Println("AdminFailover: fail-over error " + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logit.Info.Println("AdminFailover: fail-over output " + output)

	//update the old master to standalone role
	oldMaster := admindb.DBClusterNode{}
	oldMaster, err = admindb.GetDBNodeMaster(dbNode.ClusterID)
	if err != nil {
		logit.Error.Println("AdminFailover:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	oldMaster.Role = "standalone"
	oldMaster.ClusterID = "-1"
	err = admindb.UpdateDBNode(oldMaster)
	if err != nil {
		logit.Error.Println("AdminFailover:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	//update the failover node to master role
	dbNode.Role = "master"
	err = admindb.UpdateDBNode(dbNode)
	if err != nil {
		logit.Error.Println("AdminFailover:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	//stop pg on the old master
	//params.IPAddress1 = oldMaster.IPAddress

	output, err = cpmagent.AgentCommand(CPMBIN+"stoppg.sh", oldMaster.Name, oldMaster.Name)
	if err != nil {
		logit.Error.Println("AdminFailover: " + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	clusterNodes, err := admindb.GetAllDBNodesForCluster(dbNode.ClusterID)
	if err != nil {
		logit.Error.Println("AdminFailover:" + err.Error())
		rest.Error(w, err.Error(), http.StatusBadRequest)
	}

	i := 0
	for i = range clusterNodes {

		if clusterNodes[i].Name == oldMaster.Name {
			logit.Info.Println("AdminFailover: fail-over is skipping previous master")
		} else if clusterNodes[i].Name == dbNode.Name {
			logit.Info.Println("fail-over is skipping new master " + clusterNodes[i].Name)
		} else {
			if clusterNodes[i].Image == "cpm-pgpool" {
				logit.Info.Println("AdminFailover: fail-over is reconfiguring pgpool  " + clusterNodes[i].Name)
				//reconfigure pgpool node
			} else {
				//reconfigure other standby nodes
				logit.Info.Println("AdminFailover: fail-over is reconfiguring standby  " + clusterNodes[i].Name)
				//stop standby
				var commandoutput string
				commandoutput, err = PGCommand(CPMBIN+"stoppg.sh", clusterNodes[i].Name)
				if err != nil {
					logit.Error.Println("AdminFailover:" + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover: fail-over stop output was" + commandoutput)

				var domainname admindb.DBSetting
				domainname, err = admindb.GetDBSetting("DOMAIN-NAME")
				if err != nil {
					logit.Error.Println("configureCluster: DOMAIN-NAME err " + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
				}
				//create base backup from master
				commandoutput, err = cpmagent.Command1(CPMBIN+"basebackup.sh", dbNode.Name+"."+domainname.Value, clusterNodes[i].Name)
				if err != nil {
					logit.Error.Println("AdminFailover:" + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover: fail-over basebackup output was" + commandoutput)

				var data string
				data, err = template.Recovery(dbNode.Name, "5432", "postgres")
				if err != nil {
					logit.Error.Println("AdminFailover:" + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover:fail-over\t standby recovery.conf generated")

				//write standby recovery.conf file remotely
				err = RemoteWritefile("/pgdata/recovery.conf", data, clusterNodes[i].Name)
				if err != nil {
					logit.Error.Println("AdminFailover:" + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover: fail-over standby recovery.conf copied remotely")

				if cluster.ClusterType == "synchronous" {
					data, err = template.Postgresql("standby", "5432", "*")
				} else {
					data, err = template.Postgresql("standby", "5432", "")
				}
				if err != nil {
					logit.Error.Println("AdminFailover: " + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				//write standby postgresql.conf file remotely
				err = RemoteWritefile("/pgdata/postgresql.conf", data, clusterNodes[i].Name)
				if err != nil {
					logit.Error.Println("AdminFailover: " + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover: standby postgresql.conf copied remotely")

				//configure standby pg_hba.conf file
				data, err = template.Hba(KubeEnv, "standby", clusterNodes[i].Name, "5432", dbNode.ClusterID, domainname.Value)
				if err != nil {
					logit.Error.Println("AdminFailover:" + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				logit.Info.Println("AdminFailover: fail-over\t standby pg_hba.conf generated")

				//write standby pg_hba.conf file remotely
				err = RemoteWritefile("/pgdata/pg_hba.conf", data, clusterNodes[i].Name)
				if err != nil {
					logit.Error.Println("AdminFailover: " + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover:  standby pg_hba.conf copied remotely")

				//start standby

				commandoutput, err = PGCommand(CPMBIN+"startpgonstandby.sh", clusterNodes[i].Name)
				if err != nil {
					logit.Error.Println("AdminFailover:" + err.Error())
					rest.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				logit.Info.Println("AdminFailover: standby startpg output was" + commandoutput)
			}
		}

		i++
	}

	w.WriteHeader(http.StatusOK)
	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteJson(&status)
}

func EventJoinCluster(w rest.ResponseWriter, r *rest.Request) {
	err := secimpl.Authorize(r.PathParam("Token"), "perm-cluster")
	if err != nil {
		logit.Error.Println("EventJoinCluster: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	IDList := r.PathParam("IDList")
	if IDList == "" {
		logit.Error.Println("EventJoinCluster: error IDList required")
		rest.Error(w, "IDList required", http.StatusBadRequest)
		return
	} else {
		logit.Info.Println("EventJoinCluster: IDList=[" + IDList + "]")
	}

	MasterID := r.PathParam("MasterID")
	if MasterID == "" {
		logit.Error.Println("EventJoinCluster: error MasterID required")
		rest.Error(w, "MasterID required", http.StatusBadRequest)
		return
	} else {
		logit.Info.Println("EventJoinCluster: MasterID=[" + MasterID + "]")
	}
	ClusterID := r.PathParam("ClusterID")
	if ClusterID == "" {
		logit.Error.Println("EventJoinCluster: error ClusterID required")
		rest.Error(w, "node ClusterID required", http.StatusBadRequest)
		return
	} else {
		logit.Info.Println("EventJoinCluster: ClusterID=[" + ClusterID + "]")
	}

	var idList = strings.Split(IDList, "_")
	i := 0
	pgpoolCount := 0

	origDBNode := admindb.DBClusterNode{}
	for i = range idList {
		if idList[i] != "" {
			logit.Info.Println("EventJoinCluster: idList[" + strconv.Itoa(i) + "]=" + idList[i])
			origDBNode, err = admindb.GetDBNode(idList[i])
			if err != nil {
				logit.Error.Println("EventJoinCluster:" + err.Error())
				rest.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			//update the node to be in the cluster
			origDBNode.ClusterID = ClusterID
			if origDBNode.Image == "cpm-node" {
				origDBNode.Role = "standby"
			} else {
				origDBNode.Role = "pgpool"
				pgpoolCount++
			}

			if pgpoolCount > 1 {
				logit.Error.Println("EventJoinCluster: more than 1 pgpool is in the cluster")
				rest.Error(w, "only 1 pgpool is allowed in a cluster", http.StatusBadRequest)
				return
			}

			err = admindb.UpdateDBNode(origDBNode)
			if err != nil {
				logit.Error.Println("EventJoinCluster:" + err.Error())
				rest.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		i++
	}

	//we use the -1 value to indicate that we are only adding
	//to an existing cluster, the UI doesn't know who the master
	//is at this point
	if MasterID != "-1" {
		//update the master node
		origDBNode, err = admindb.GetDBNode(MasterID)
		if err != nil {
			logit.Error.Println("EventJoinCluster:" + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		origDBNode.ClusterID = ClusterID
		origDBNode.Role = "master"
		err = admindb.UpdateDBNode(origDBNode)
		if err != nil {
			logit.Error.Println("EventJoinCluster:" + err.Error())
			rest.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteJson(&status)
}

func AutoCluster(w rest.ResponseWriter, r *rest.Request) {
	logit.Info.Println("AUTO CLUSTER PROFILE starts")
	logit.Info.Println("AutoCluster: start AutoCluster")
	params := AutoClusterInfo{}
	err := r.DecodeJsonPayload(&params)
	if err != nil {
		logit.Error.Println("AutoCluster: error in decode" + err.Error())
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = secimpl.Authorize(params.Token, "perm-cluster")
	if err != nil {
		logit.Error.Println("AutoCluster: authorize error " + err.Error())
		rest.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if params.Name == "" {
		logit.Error.Println("AutoCluster: error in Name")
		rest.Error(w, "cluster name required", http.StatusBadRequest)
		return
	}
	if params.ClusterType == "" {
		logit.Error.Println("AutoCluster: error in ClusterType")
		rest.Error(w, "ClusterType name required", http.StatusBadRequest)
		return
	}
	if params.ClusterProfile == "" {
		logit.Error.Println("AutoCluster: error in ClusterProfile")
		rest.Error(w, "ClusterProfile name required", http.StatusBadRequest)
		return
	}

	logit.Info.Println("AutoCluster: Name=" + params.Name + " ClusterType=" + params.ClusterType + " Profile=" + params.ClusterProfile)

	//create cluster definition
	dbcluster := admindb.DBCluster{"", params.Name, params.ClusterType, "uninitialized", ""}
	var ival int
	ival, err = admindb.InsertDBCluster(dbcluster)
	clusterID := strconv.Itoa(ival)
	dbcluster.ID = clusterID
	logit.Info.Println(clusterID)
	if err != nil {
		logit.Error.Println("AutoCluster:" + err.Error())
		rest.Error(w, "Insert Cluster error:"+err.Error(), http.StatusBadRequest)
		return
	}

	//lookup profile
	profile, err2 := getClusterProfileInfo(params.ClusterProfile)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-" + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	var masterServer admindb.DBServer
	var chosenServers []admindb.DBServer
	if profile.Algo == "round-robin" {
		masterServer, chosenServers, err2 = roundRobin(profile)
	} else {
		logit.Error.Println("AutoCluster: error-unsupported algorithm request")
		rest.Error(w, "AutoCluster error: unsupported algorithm", http.StatusBadRequest)
		return
	}

	//create master container
	dockermaster := cpmagent.DockerRunArgs{}
	dockermaster.Image = "crunchydata/cpm-node"
	dockermaster.ContainerName = params.Name + "-master"
	dockermaster.ServerID = masterServer.ID
	dockermaster.Standalone = "false"
	if err != nil {
		logit.Error.Println("AutoCluster: error-create master node " + err.Error())
		rest.Error(w, "AutoCluster error"+err.Error(), http.StatusBadRequest)
		return
	}

	//	provision the master
	err2 = provisionImpl(&dockermaster, profile.MasterProfile, false)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-provision master " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	logit.Info.Println("AUTO CLUSTER PROFILE master container created")
	var node admindb.DBClusterNode
	//update node with cluster iD
	node, err2 = admindb.GetDBNodeByName(dockermaster.ContainerName)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-get node by name " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	node.ClusterID = clusterID
	node.Role = "master"
	err2 = admindb.UpdateDBNode(node)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-update standby node " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	//create standby containers
	var count int
	count, err2 = strconv.Atoi(profile.Count)
	if err2 != nil {
		logit.Error.Println(err2.Error())
		rest.Error(w, err2.Error(), http.StatusBadRequest)
		return
	}

	dockerstandby := make([]cpmagent.DockerRunArgs, count)
	for i := 0; i < count; i++ {
		logit.Info.Println("working on standby ....")
		//	loop - provision standby
		dockerstandby[i].ServerID = chosenServers[i].ID
		dockerstandby[i].Image = "crunchydata/cpm-node"
		dockerstandby[i].ContainerName = params.Name + "-standby-" + strconv.Itoa(i)
		dockerstandby[i].Standalone = "false"
		err2 = provisionImpl(&dockerstandby[i], profile.StandbyProfile, true)
		if err2 != nil {
			logit.Error.Println("AutoCluster: error-provision master " + err2.Error())
			rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
			return
		}

		//update node with cluster iD
		node, err2 = admindb.GetDBNodeByName(dockerstandby[i].ContainerName)
		if err2 != nil {
			logit.Error.Println("AutoCluster: error-get node by name " + err2.Error())
			rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
			return
		}

		node.ClusterID = clusterID
		node.Role = "standby"
		err2 = admindb.UpdateDBNode(node)
		if err2 != nil {
			logit.Error.Println("AutoCluster: error-update standby node " + err2.Error())
			rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
			return
		}
	}
	logit.Info.Println("AUTO CLUSTER PROFILE standbys created")
	//create pgpool container
	//	provision
	dockerpgpool := cpmagent.DockerRunArgs{}
	dockerpgpool.ContainerName = params.Name + "-pgpool"
	dockerpgpool.Image = "crunchydata/cpm-pgpool"
	dockerpgpool.ServerID = chosenServers[count].ID
	dockerpgpool.Standalone = "false"

	err2 = provisionImpl(&dockerpgpool, profile.StandbyProfile, true)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-provision pgpool " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}
	logit.Info.Println("AUTO CLUSTER PROFILE pgpool created")
	//update node with cluster ID
	node, err2 = admindb.GetDBNodeByName(dockerpgpool.ContainerName)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-get pgpool node by name " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	node.ClusterID = clusterID
	node.Role = "pgpool"
	err2 = admindb.UpdateDBNode(node)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-update pgpool node " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	//init the master DB
	//	provision the master
	err2 = provisionImplInit(&dockermaster, profile.MasterProfile, false)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-provisionInit master " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	//make sure every node is ready
	err2 = waitTillAllReady(dockermaster, dockerpgpool, dockerstandby)
	if err2 != nil {
		logit.Error.Println("cluster members not responding in time")
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	//configure cluster
	//	ConfigureCluster
	logit.Info.Println("AUTO CLUSTER PROFILE configure cluster ")
	err2 = configureCluster(dbcluster, true)
	if err2 != nil {
		logit.Error.Println("AutoCluster: error-configure cluster " + err2.Error())
		rest.Error(w, "AutoCluster error"+err2.Error(), http.StatusBadRequest)
		return
	}

	logit.Info.Println("AUTO CLUSTER PROFILE done")
	w.WriteHeader(http.StatusOK)
	status := SimpleStatus{}
	status.Status = "OK"
	w.WriteJson(&status)
}

// round-robin provisioning algorithm -
//  to promote least used servers, incoming servers list
//  should be sorted by class and least used order
//  returns the master server and the list of standby servers
func roundRobin(profile ClusterProfiles) (admindb.DBServer, []admindb.DBServer, error) {
	var masterServer admindb.DBServer
	count, err := strconv.Atoi(profile.Count)

	//add 1 to the standby count to make room for the pgpool node
	count++

	//create a slice to hold servers for standby and pgpool nodes
	//assumes 1 pgpool node per cluster which is enforced by auto-cluster
	chosen := make([]admindb.DBServer, count)

	//get all the servers available
	servers, err := admindb.GetAllDBServersByClassByCount()
	if err != nil {
		return masterServer, chosen, err
	}
	if len(servers) == 0 {
		return masterServer, chosen, errors.New("no servers defined")
	}

	//find the server for the master
	//search from last used to end of servers list
	found := false
	for j := 0; j < len(servers); j++ {
		if profile.MasterServer == servers[j].ServerClass {
			found = true
			masterServer = servers[j]
			break
		}
	}

	//give up on finding a match and use any server
	if !found {
		for j := 0; j < len(servers); j++ {
			masterServer = servers[j]
			break
		}
	}

	//find the servers for all the other nodes (standby, pgpool)
	//avoiding the use of the masterServer for HA

	lastused := 0

	for i := 0; i < count; i++ {

		found = false

		//search from last used to end of servers list
		for j := lastused; j < len(servers); j++ {
			if servers[j].ID != masterServer.ID &&
				servers[j].ServerClass == profile.StandbyServer {
				chosen[i] = servers[j]
				found = true
				lastused = j
				break
			}
		}

		if !found {
			//search from start of servers list to end
			for j := 0; j < len(servers); j++ {
				if servers[j].ID != masterServer.ID && servers[j].ServerClass == profile.StandbyServer {
					chosen[i] = servers[j]
					found = true
					lastused = j
					break
				}
			}

		}

		//if still not found, use any server
		if !found {
			//search from start of servers list to end
			for j := 0; j < len(servers); j++ {
				chosen[i] = servers[j]
				found = true
				lastused = j
				break
			}

		}

	}

	logit.Info.Println("round-robin: master " + masterServer.Name + " class=" + masterServer.ServerClass)
	for x := 0; x < len(chosen); x++ {
		logit.Info.Println("round-robin: other servers " + chosen[x].Name + " class=" + chosen[x].ServerClass)
	}
	return masterServer, chosen, nil
}

func waitTillAllReady(dockermaster cpmagent.DockerRunArgs, dockerpgpool cpmagent.DockerRunArgs, dockerstandby []cpmagent.DockerRunArgs) error {
	err := waitTillReady(dockermaster.ContainerName)
	if err != nil {
		logit.Error.Println("time out waiting for " + dockermaster.ContainerName)
		return err
	}
	err = waitTillReady(dockerpgpool.ContainerName)
	if err != nil {
		logit.Error.Println("time out waiting for " + dockerpgpool.ContainerName)
		return err
	}
	for x := 0; x < len(dockerstandby); x++ {
		err = waitTillReady(dockerstandby[x].ContainerName)
		if err != nil {
			logit.Error.Println("time out waiting for " + dockerstandby[x].ContainerName)
			return err
		}
	}
	return nil

}