/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package migrationscripts

import (
	"context"
	"github.com/apache/incubator-devlake/plugins/helper"
	"gorm.io/gorm"
)

type JiraConnection20220524 struct {
	helper.RestConnection
	helper.BasicAuth
	EpicKeyField               string `gorm:"type:varchar(50);" json:"epicKeyField"`
	StoryPointField            string `gorm:"type:varchar(50);" json:"storyPointField"`
	RemotelinkCommitShaPattern string `gorm:"type:varchar(255);comment='golang regexp, the first group will be recognized as commit sha, ref https://github.com/google/re2/wiki/Syntax'" json:"remotelinkCommitShaPattern"`
}

func (JiraConnection20220524) TableName() string {
	return "_tool_jira_connections"
}

type UpdateSchemas20220524 struct{}

func (*UpdateSchemas20220524) Up(ctx context.Context, db *gorm.DB) error {
	var err error
	if !db.Migrator().HasColumn(&JiraConnection20220505{}, "password") {
		err = db.Migrator().AddColumn(&JiraConnection20220524{}, "password")
		if err != nil {
			return err
		}
	}

	if !db.Migrator().HasColumn(&JiraConnection20220505{}, "username") {
		err = db.Migrator().AddColumn(&JiraConnection20220524{}, "username")
		if err != nil {
			return err
		}
	}

	if db.Migrator().HasColumn(&JiraConnection20220505{}, "basic_auth_encoded") {
		err = db.Migrator().DropColumn(&JiraConnection20220505{}, "basic_auth_encoded")
		if err != nil {
			return err
		}
	}

	return nil
}

func (*UpdateSchemas20220524) Version() uint64 {
	return 20220507154646
}

func (*UpdateSchemas20220524) Name() string {
	return "Add icon_url column to JiraIssue"
}