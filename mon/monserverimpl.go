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

package mon

import (
	"github.com/crunchydata/crunchy-postgresql-manager/logit"
	"github.com/robfig/cron"
)

type Command struct {
	Output string
}

type MonRequest struct {
	Schedule MonSchedule
}

//global cron instance that gets started, stopped, restarted
var CRONInstance *cron.Cron

//placeholder for a client call to the monitor server if ever needed
func (t *Command) Placeholder(status *string, reply *Command) error {

	logit.Info.Println("Placeholder called")
	*status = "processed on monitor server"
	return nil
}

func LoadSchedules() error {

	var err error
	logit.Info.Println("LoadSchedules called")

	schedules, err := DBGetSchedules()
	if err != nil {
		logit.Error.Println("LoadSchedules error " + err.Error())
	}

	if CRONInstance != nil {
		logit.Info.Println("stopping current cron instance...")
		CRONInstance.Stop()
	}

	//kill off the old cron, garbage collect it
	CRONInstance = nil

	//create a new cron
	logit.Info.Println("creating cron instance...")
	CRONInstance = cron.New()

	for i := 0; i < len(schedules); i++ {
		logit.Info.Println("schedule added " + schedules[i].Name)
		x := DefaultJob{}
		x.request = MonRequest{}
		x.request.Schedule = schedules[i]
		CRONInstance.AddJob(schedules[i].Cronexp, x)
	}

	logit.Info.Println("starting new CRONInstance")
	CRONInstance.Start()

	return err
}
