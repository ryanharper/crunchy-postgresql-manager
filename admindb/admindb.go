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

package admindb

import (
	"database/sql"
	"fmt"
	"github.com/crunchydata/crunchy-postgresql-manager/logit"
	"github.com/crunchydata/crunchy-postgresql-manager/sec"
	_ "github.com/lib/pq"
	"strconv"
	"strings"
)

type Setting struct {
	Name       string
	Value      string
	UpdateDate string
}
type Server struct {
	ID             string
	Name           string
	IPAddress      string
	DockerBridgeIP string
	PGDataPath     string
	ServerClass    string
	CreateDate     string
	NodeCount      string
}

type Cluster struct {
	ID          string
	Name        string
	ClusterType string
	Status      string
	CreateDate  string
}

type Container struct {
	ID         string
	ClusterID  string
	ServerID   string
	Name       string
	Role       string
	Image      string
	CreateDate string
}

type ContainerUser struct {
	ID            string
	Containername string
	Usename       string
	Passwd        string
	UpdateDate    string
}

type LinuxStats struct {
	ID        string
	ClusterID string
	Stats     string
}

type PGStats struct {
	ID        string
	ClusterID string
	Stats     string
}

var dbConn *sql.DB

func SetConnection(conn *sql.DB) {
	dbConn = conn
}

func GetServer(id string) (Server, error) {
	//logit.Info.Println("GetServer called with id=" + id)
	server := Server{}

	err := dbConn.QueryRow(fmt.Sprintf("select id, name, ipaddress, dockerbip, pgdatapath, serverclass, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from server where id=%s", id)).Scan(&server.ID, &server.Name, &server.IPAddress, &server.DockerBridgeIP, &server.PGDataPath, &server.ServerClass, &server.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetServer:no server with that id")
		return server, err
	case err != nil:
		logit.Info.Println("admindb:GetServer:" + err.Error())
		return server, err
	default:
		logit.Info.Println("admindb:GetServer: server name returned is " + server.Name)
	}

	return server, nil
}

func GetCluster(id string) (Cluster, error) {
	//logit.Info.Println("admindb:GetCluster: called")
	cluster := Cluster{}

	err := dbConn.QueryRow(fmt.Sprintf("select id, name, clustertype, status, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from cluster where id=%s", id)).Scan(&cluster.ID, &cluster.Name, &cluster.ClusterType, &cluster.Status, &cluster.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetCluster:no cluster with that id")
		return cluster, err
	case err != nil:
		logit.Info.Println("admindb:GetCluster:" + err.Error())
		return cluster, err
	default:
		logit.Info.Println("admindb:GetCluster: cluster name returned is " + cluster.Name)
	}

	return cluster, nil
}

func GetAllClusters() ([]Cluster, error) {
	//logit.Info.Println("admindb:GetAllClusters: called")
	var rows *sql.Rows
	var err error
	rows, err = dbConn.Query("select id, name, clustertype, status, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from cluster order by name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	clusters := make([]Cluster, 0)
	for rows.Next() {
		cluster := Cluster{}
		if err = rows.Scan(
			&cluster.ID,
			&cluster.Name,
			&cluster.ClusterType,
			&cluster.Status, &cluster.CreateDate); err != nil {
			return nil, err
		}
		clusters = append(clusters, cluster)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return clusters, nil
}

func UpdateCluster(cluster Cluster) error {
	//logit.Info.Println("admindb:UpdateCluster:called")
	queryStr := fmt.Sprintf("update cluster set ( name, clustertype, status) = ('%s', '%s', '%s') where id = %s returning id", cluster.Name, cluster.ClusterType, cluster.Status, cluster.ID)

	logit.Info.Println("admindb:UpdateCluster:update str=[" + queryStr + "]")
	var clusterid int
	err := dbConn.QueryRow(queryStr).Scan(&clusterid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:UpdateCluster:cluster updated " + cluster.ID)
	}
	return nil

}
func InsertCluster(cluster Cluster) (int, error) {
	//logit.Info.Println("admindb:InsertCluster:called")
	queryStr := fmt.Sprintf("insert into cluster ( name, clustertype, status, createdt) values ( '%s', '%s', '%s', now()) returning id", cluster.Name, cluster.ClusterType, cluster.Status)

	logit.Info.Println("admindb:InsertCluster:" + queryStr)
	var clusterid int
	err := dbConn.QueryRow(queryStr).Scan(&clusterid)
	switch {
	case err != nil:
		logit.Info.Println("admindb:InsertCluster:" + err.Error())
		return -1, err
	default:
		logit.Info.Println("admindb:InsertCluster: cluster inserted returned is " + strconv.Itoa(clusterid))
	}

	return clusterid, nil
}

func DeleteCluster(id string) error {
	queryStr := fmt.Sprintf("delete from cluster where  id=%s returning id", id)
	//logit.Info.Println("admindb:DeleteCluster:" + queryStr)

	var clusterid int
	err := dbConn.QueryRow(queryStr).Scan(&clusterid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:DeleteCluster:cluster deleted " + id)
	}
	return nil
}

func GetContainer(id string) (Container, error) {
	//logit.Info.Println("admindb:GetContainer:called")
	container := Container{}

	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where id=%s", id)
	err := dbConn.QueryRow(queryStr).Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetContainer:no container with that id " + id)
		return container, err
	case err != nil:
		return container, err
	}

	return container, nil
}

func GetContainerByName(name string) (Container, error) {
	//logit.Info.Println("admindb:GetNodeByName:called")
	container := Container{}

	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where name='%s'", name)
	err := dbConn.QueryRow(queryStr).Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetContainerByName:no container with that name " + name)
		return container, err
	case err != nil:
		return container, err
	}

	return container, nil
}

//find the oldest container in a cluster, used for serf join-cluster event
func GetContainerOldestInCluster(clusterid string) (Container, error) {
	//logit.Info.Println("admindb:GetNodeOldestInCluster:called")
	container := Container{}

	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where createdt = (select max(createdt) from container where clusterid = %s)", clusterid)
	logit.Info.Println("admindb:GetNodeOldestInCluster:" + queryStr)
	err := dbConn.QueryRow(queryStr).Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetContainerOldestInCluster: no container with that clusterid " + clusterid)
		return container, err
	case err != nil:
		return container, err
	}

	return container, nil
}

//find the master container in a cluster, used for serf fail-over event
func GetContainerMaster(clusterid string) (Container, error) {
	//logit.Info.Println("admindb:GetContainerMaster:called")
	container := Container{}

	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where role = 'master' and clusterid = %s", clusterid)
	logit.Info.Println("admindb:GetContainerMaster:" + queryStr)
	err := dbConn.QueryRow(queryStr).Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetContainerMaster: no master container with that clusterid " + clusterid)
		return container, err
	case err != nil:
		return container, err
	}

	return container, nil
}

//find the pgpool container in a cluster
func GetContainerPgpool(clusterid string) (Container, error) {
	//logit.Info.Println("admindb:GetContainerMaster:called")
	container := Container{}

	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where role = 'pgpool' and clusterid = %s", clusterid)
	logit.Info.Println("admindb:GetContainerMaster:" + queryStr)
	err := dbConn.QueryRow(queryStr).Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetContainerMaster: no pgpool container with that clusterid " + clusterid)
		return container, err
	case err != nil:
		return container, err
	}

	return container, nil
}

//
// TODO combine with GetMaster into a GetContainersByRole func
//
func GetAllStandbyContainers(clusterid string) ([]Container, error) {
	var rows *sql.Rows
	var err error
	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where role = 'standby' and clusterid = %s", clusterid)
	logit.Info.Println("admindb:GetAllStandbyContainers:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	containers := make([]Container, 0)
	for rows.Next() {
		container := Container{}
		if err = rows.Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate); err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return containers, nil
}

func GetAllContainersForServer(serverID string) ([]Container, error) {
	var rows *sql.Rows
	var err error
	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where serverid = %s order by name", serverID)
	logit.Info.Println("admindb:GetAllContainersForServer:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	containers := make([]Container, 0)
	for rows.Next() {
		container := Container{}
		if err = rows.Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate); err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return containers, nil
}

func GetAllContainersForCluster(clusterID string) ([]Container, error) {
	var rows *sql.Rows
	var err error
	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where clusterid = %s order by name", clusterID)
	logit.Info.Println("admindb:GetAllContainersForCluster:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	containers := make([]Container, 0)
	for rows.Next() {
		container := Container{}
		if err = rows.Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate); err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return containers, nil
}

//
// GetAllContainersNotInCluster is used to fetch all nodes that
// are eligible to be added into a cluster
//
func GetAllContainersNotInCluster() ([]Container, error) {
	var rows *sql.Rows
	var err error
	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container where role != 'standalone' and clusterid = -1 order by name")
	logit.Info.Println("admindb:GetAllContainersNotInCluster:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	containers := make([]Container, 0)
	for rows.Next() {
		container := Container{}
		if err = rows.Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate); err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return containers, nil
}

func GetAllContainers() ([]Container, error) {
	var rows *sql.Rows
	var err error
	queryStr := fmt.Sprintf("select id, name, clusterid, serverid, role, image, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from container order by name")
	logit.Info.Println("admindb:GetAllContainers:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	containers := make([]Container, 0)
	for rows.Next() {
		container := Container{}
		if err = rows.Scan(&container.ID, &container.Name, &container.ClusterID, &container.ServerID, &container.Role, &container.Image, &container.CreateDate); err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return containers, nil
}

func InsertContainer(container Container) (int, error) {
	queryStr := fmt.Sprintf("insert into container ( name, clusterid, serverid, role, image, createdt) values ( '%s', %s, %s, '%s','%s', now()) returning id", container.Name, container.ClusterID, container.ServerID, container.Role, container.Image)

	logit.Info.Println("admindb:InsertContainer:" + queryStr)
	var nodeid int
	err := dbConn.QueryRow(queryStr).Scan(&nodeid)
	switch {
	case err != nil:
		logit.Info.Println("admindb:InsertContainer:" + err.Error())
		return -1, err
	default:
		logit.Info.Println("admindb:InsertContainer:container inserted returned is " + strconv.Itoa(nodeid))
	}

	return nodeid, nil
}

func DeleteContainer(id string) error {
	queryStr := fmt.Sprintf("delete from container where  id=%s returning id", id)
	logit.Info.Println("admindb:DeleteContainer:" + queryStr)

	var nodeid int
	err := dbConn.QueryRow(queryStr).Scan(&nodeid)
	switch {
	case err != nil:
		logit.Error.Println(err)
		return err
	default:
		logit.Info.Println("admindb:DeleteContainer:cluster deleted " + id)
	}
	return nil
}

func UpdateContainer(container Container) error {
	queryStr := fmt.Sprintf("update container set ( name, clusterid, serverid, role, image) = ('%s', %s, %s, '%s', '%s') where id = %s returning id", container.Name, container.ClusterID, container.ServerID, container.Role, container.Image, container.ID)
	logit.Info.Println("admindb:UpdateContainer:" + queryStr)

	var nodeid int
	err := dbConn.QueryRow(queryStr).Scan(&nodeid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:UpdateContainer: container updated " + container.Name)
	}
	return nil

}

func GetAllServers() ([]Server, error) {
	logit.Info.Println("admindb:GetAllServer:called")
	var rows *sql.Rows
	var err error
	rows, err = dbConn.Query("select id, name, ipaddress, dockerbip, pgdatapath, serverclass, to_char(createdt, 'MM-DD-YYYY HH24:MI:SS') from server order by name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := make([]Server, 0)
	for rows.Next() {
		server := Server{}
		if err = rows.Scan(&server.ID, &server.Name,
			&server.IPAddress, &server.DockerBridgeIP, &server.PGDataPath, &server.ServerClass, &server.CreateDate); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return servers, nil
}

func GetAllServersByClassByCount() ([]Server, error) {
	//select s.id, s.name, s.serverclass, count(n) from server s left join node n on  s.id = n.serverid  group by s.id order by s.serverclass, count(n);

	logit.Info.Println("admindb:GetAllServerByClassByCount:called")
	var rows *sql.Rows
	var err error
	rows, err = dbConn.Query("select s.id, s.name, s.ipaddress, s.dockerbip, s.pgdatapath, s.serverclass, to_char(s.createdt, 'MM-DD-YYYY HH24:MI:SS'), count(n) from server s left join container n on s.id = n.serverid group by s.id  order by s.serverclass, count(n)")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := make([]Server, 0)
	for rows.Next() {
		server := Server{}
		if err = rows.Scan(&server.ID, &server.Name,
			&server.IPAddress, &server.DockerBridgeIP, &server.PGDataPath, &server.ServerClass, &server.CreateDate, &server.NodeCount); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return servers, nil
}

func UpdateServer(server Server) error {
	logit.Info.Println("admindb:UpdateServer:called")
	queryStr := fmt.Sprintf("update server set ( name, ipaddress, pgdatapath, serverclass, dockerbip) = ('%s', '%s', '%s', '%s', '%s') where id = %s returning id", server.Name, server.IPAddress, server.PGDataPath, server.ServerClass, server.DockerBridgeIP, server.ID)

	logit.Info.Println("admindb:UpdateServer:update str=" + queryStr)
	var serverid int
	err := dbConn.QueryRow(queryStr).Scan(&serverid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:UpdateServer:server updated " + server.ID)
	}
	return nil

}
func InsertServer(server Server) (int, error) {
	//logit.Info.Println("admindb:InsertServer:called")
	queryStr := fmt.Sprintf("insert into server ( name, ipaddress, pgdatapath, serverclass, dockerbip, createdt) values ( '%s', '%s', '%s', '%s', '%s', now()) returning id", server.Name, server.IPAddress, server.PGDataPath, server.ServerClass, server.DockerBridgeIP)

	logit.Info.Println("admindb:InsertServer:" + queryStr)
	var serverid int
	err := dbConn.QueryRow(queryStr).Scan(&serverid)
	switch {
	case err != nil:
		logit.Info.Println("admindb:InsertServer:" + err.Error())
		return -1, err
	default:
		logit.Info.Println("admindb:InsertServer: server inserted returned is " + strconv.Itoa(serverid))
	}

	return serverid, nil
}

func DeleteServer(id string) error {
	queryStr := fmt.Sprintf("delete from server where  id=%s returning id", id)
	logit.Info.Println("admindb:DeleteServer:" + queryStr)

	var serverid int
	err := dbConn.QueryRow(queryStr).Scan(&serverid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:DeleteServer:server deleted " + id)
	}
	return nil
}

func GetAllSettings() ([]Setting, error) {
	//logit.Info.Println("admindb:GetAllSettings: called")
	var rows *sql.Rows
	var err error
	rows, err = dbConn.Query("select name, value, to_char(updatedt, 'MM-DD-YYYY HH24:MI:SS') from settings order by name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]Setting, 0)
	for rows.Next() {
		setting := Setting{}
		if err = rows.Scan(
			&setting.Name,
			&setting.Value,
			&setting.UpdateDate); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

func GetSetting(key string) (Setting, error) {
	//logit.Info.Println("admindb:GetSetting:called")
	setting := Setting{}

	queryStr := fmt.Sprintf("select value, to_char(updatedt, 'MM-DD-YYYY HH24:MI:SS') from settings where name = '%s'", key)
	//logit.Info.Println("admindb:GetSetting:" + queryStr)
	err := dbConn.QueryRow(queryStr).Scan(&setting.Value, &setting.UpdateDate)
	switch {
	case err == sql.ErrNoRows:
		logit.Info.Println("admindb:GetSetting: no Setting with that key " + key)
		return setting, err
	case err != nil:
		return setting, err
	}

	return setting, nil
}

func InsertSetting(setting Setting) error {
	logit.Info.Println("admindb:InsertSetting:called")
	queryStr := fmt.Sprintf("insert into setting ( name, value, createdt) values ( '%s', '%s', now()) returning name", setting.Name, setting.Value)

	logit.Info.Println("admindb:InsertSetting:" + queryStr)
	var name string
	err := dbConn.QueryRow(queryStr).Scan(&name)
	switch {
	case err != nil:
		logit.Info.Println("admindb:InsertSetting:" + err.Error())
		return err
	default:
	}

	return nil
}

func UpdateSetting(setting Setting) error {
	logit.Info.Println("admindb:UpdateSetting:called")
	queryStr := fmt.Sprintf("update settings set ( value, updatedt) = ('%s', now()) where name = '%s'  returning name", setting.Value, setting.Name)

	logit.Info.Println("admindb:UpdateSetting:update str=[" + queryStr + "]")
	var name string
	err := dbConn.QueryRow(queryStr).Scan(&name)
	switch {
	case err != nil:
		logit.Info.Println("admindb:UpdateSetting:" + err.Error())
		return err
	default:
	}
	return nil

}

func GetAllSettingsMap() (map[string]string, error) {
	logit.Info.Println("admindb:GetAllSettingsMap: called")
	m := make(map[string]string)

	var rows *sql.Rows
	var err error
	rows, err = dbConn.Query("select name, value, to_char(updatedt, 'MM-DD-YYYY HH24:MI:SS') from settings order by name")
	if err != nil {
		return m, err
	}
	defer rows.Close()
	//settings := make([]Setting, 0)
	for rows.Next() {
		setting := Setting{}
		if err = rows.Scan(
			&setting.Name,
			&setting.Value,
			&setting.UpdateDate); err != nil {
			return m, err
		}
		m[setting.Name] = setting.Value
		//settings = append(settings, setting)
	}
	if err = rows.Err(); err != nil {
		return m, err
	}
	return m, nil
}

func Test() {
	logit.Info.Println("hi from Test")
}

func GetDomain() (string, error) {
	tmp, err := GetSetting("DOMAIN-NAME")

	if err != nil {
		return "", err
	}
	//we trim off any leading . characters
	domain := strings.Trim(tmp.Value, ".")

	return domain, nil
}

func AddContainerUser(s ContainerUser) (int, error) {

	//logit.Info.Println("AddContainerUser called")

	//encrypt the password...passwords at rest are encrypted
	encrypted, err := sec.EncryptPassword(s.Passwd)

	queryStr := fmt.Sprintf("insert into containeruser ( containername, usename, passwd, updatedt) values ( '%s', '%s', '%s',  now()) returning id",
		s.Containername,
		s.Usename,
		encrypted)

	logit.Info.Println("AddContainerUser:" + queryStr)
	var theID int
	err = dbConn.QueryRow(queryStr).Scan(
		&theID)
	if err != nil {
		logit.Error.Println("error in AddContainerUser query " + err.Error())
		return theID, err
	}

	switch {
	case err != nil:
		logit.Error.Println("AddContainerUser: error " + err.Error())
		return theID, err
	default:
	}

	return theID, nil
}

func DeleteContainerUser(id string) error {
	queryStr := fmt.Sprintf("delete from containeruser where id=%s returning id", id)
	//logit.Info.Println("admindb:DeleteCluster:" + queryStr)

	var nodeuserid int
	err := dbConn.QueryRow(queryStr).Scan(&nodeuserid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:DeleteContainerUser: deleted " + id)
	}
	return nil
}

func GetAllUsersForContainer(containerName string) ([]ContainerUser, error) {
	var rows *sql.Rows
	var err error
	queryStr := fmt.Sprintf("select id, usename, passwd, to_char(updatedt, 'MM-DD-YYYY HH24:MI:SS') from containeruser where containername = '%s' order by usename", containerName)
	logit.Info.Println("admindb:GetAllUsersForContainer:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]ContainerUser, 0)
	for rows.Next() {
		user := ContainerUser{}
		user.Containername = containerName
		if err = rows.Scan(&user.ID, &user.Usename, &user.Passwd, &user.UpdateDate); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func GetContainerUser(containername string, usename string) (ContainerUser, error) {
	var rows *sql.Rows
	var user ContainerUser
	var err error
	queryStr := fmt.Sprintf("select id, passwd, to_char(updatedt, 'MM-DD-YYYY HH24:MI:SS') from containeruser where usename = '%s' and containername = '%s'", usename, containername)
	logit.Info.Println("admindb:GetContainerUser:" + queryStr)
	rows, err = dbConn.Query(queryStr)
	if err != nil {
		return user, err
	}
	defer rows.Close()
	for rows.Next() {
		user.Usename = usename
		user.Containername = containername
		if err = rows.Scan(&user.ID, &user.Passwd, &user.UpdateDate); err != nil {
			return user, err
		}
	}
	if err = rows.Err(); err != nil {
		return user, err
	}
	var unencrypted string
	unencrypted, err = sec.DecryptPassword(user.Passwd)
	if err != nil {
		return user, err
	}
	user.Passwd = unencrypted
	return user, nil
}

func UpdateContainerUser(user ContainerUser) error {
	//logit.Info.Println("admindb:UpdateCluster:called")
	queryStr := fmt.Sprintf("update containeruser set ( passwd, updatedt) = ('%s', now()) where id = %s returning id", user.Passwd, user.ID)

	logit.Info.Println("[" + queryStr + "]")
	var userid int
	err := dbConn.QueryRow(queryStr).Scan(&userid)
	switch {
	case err != nil:
		return err
	default:
		logit.Info.Println("admindb:UpdateContainerUser:updated " + user.ID)
	}
	return nil

}
