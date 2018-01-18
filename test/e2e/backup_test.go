// Copyright 2018 Oracle and/or its affiliates. All rights reserved.
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

// +build all default

package e2e

import (
	"testing"

	api "github.com/oracle/mysql-operator/pkg/apis/mysql/v1"
	"github.com/oracle/mysql-operator/pkg/constants"
	"github.com/oracle/mysql-operator/test/e2e/framework"
	e2eutil "github.com/oracle/mysql-operator/test/e2e/util"
)

const testDatabaseName string = "employees"

func TestBackUpRestore(test *testing.T) {
	t := e2eutil.NewT(test)
	f := framework.Global
	var err error

	// ---------------------------------------------------------------------- //
	t.Log("Creating mysqlcluster...")
	// ---------------------------------------------------------------------- //
	testdb := e2eutil.CreateTestDB(t, "e2e-br-", 1, f.DestroyAfterFailure)
	defer testdb.Delete()
	clusterName := testdb.Cluster().Name

	testdb.Populate()
	testdb.Test()

	// ---------------------------------------------------------------------- //
	t.Logf("Creating mysqlbackup for mysqlcluster '%s'...", clusterName)
	// ---------------------------------------------------------------------- //
	backupName := "e2e-test-snapshot-backup-"
	s3StorageCredentials := "s3-upload-credentials"
	backupSpec := e2eutil.NewMySQLBackup(clusterName, backupName, s3StorageCredentials, []string{testDatabaseName})
	backup, err := f.MySQLOpClient.MysqlV1().MySQLBackups(f.Namespace).Create(backupSpec)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}
	backupBackoff := e2eutil.NewDefaultRetyWithDuration(10)
	backupBackoff.Steps = 10
	backup, err = e2eutil.WaitForBackupPhase(t, backup, api.BackupPhaseComplete, backupBackoff, f.MySQLOpClient)
	if err != nil {
		t.Fatalf("Backup failed to reach phase %q: %v", api.BackupPhaseComplete, err)
	}
	if backup.Labels[constants.MySQLOperatorVersionLabel] != f.BuildVersion {
		t.Errorf("Backup MySQLOperatorVersionLabel was incorrect: %s != %s.", backup.Labels[constants.MySQLOperatorVersionLabel], f.BuildVersion)
	} else {
		t.Logf("Backup label MySQLOperatorVersionLabel: %s", backup.Labels[constants.MySQLOperatorVersionLabel])
	}
	if backup.Spec.AgentScheduled != clusterName+"-0" {
		t.Fatalf("Backup was not scheduled on cluster master node.")
	} else {
		t.Logf("Backup AgentScheduled: %s", backup.Spec.AgentScheduled)
	}
	if backup.Status.Outcome.Location == "" {
		t.Fatalf("Backup failed to specify a location for the image archive.")
	}
	t.Logf("created backup at location: %s", backup.Status.Outcome.Location)

	// ---------------------------------------------------------------------- //
	t.Log("Trying connection to container")
	// ---------------------------------------------------------------------- //
	err = e2eutil.Retry(e2eutil.DefaultRetry, func() (bool, error) {
		passwd, err := testdb.GetPassword()
		return passwd != "", err
	})
	if err != nil {
		t.Fatalf("Failed to connect to the database")
	}

	// ---------------------------------------------------------------------- //
	t.Log("Validating database..")
	// ---------------------------------------------------------------------- //
	testdb.Test()

	// ---------------------------------------------------------------------- //
	t.Log("Deleting the %s database..", testDatabaseName)
	// ---------------------------------------------------------------------- //
	podName := clusterName + "-0"
	username := "root"
	password := e2eutil.GetMySQLPassword(t, podName, f.Namespace)
	sqlExecutor := e2eutil.NewKubectlSimpleSQLExecutor(t, podName, username, password, f.Namespace)
	dbHelper := e2eutil.NewMySQLDBTestHelper(t, sqlExecutor)
	dbHelper.DeleteDB(testDatabaseName)
	if dbHelper.HasDB(testDatabaseName) {
		t.Fatalf("Failed to delete %s database", testDatabaseName)
	}

	// ---------------------------------------------------------------------- //
	t.Logf("creating mysqlrestore from mysqlbackup '%s' for mysqlcluster '%s'.", backup.Name, clusterName)
	// ---------------------------------------------------------------------- //
	restoreName := backup.Name + "-restore-"
	restoreSpec := e2eutil.NewMySQLRestore(clusterName, backup.Name, restoreName)
	restore, err := f.MySQLOpClient.MysqlV1().MySQLRestores(f.Namespace).Create(restoreSpec)
	if err != nil {
		t.Fatalf("Failed to create restore '%s': %v", backup.Name, err)
	}
	restoreBackoff := e2eutil.NewDefaultRetyWithDuration(10)
	restoreBackoff.Steps = 24
	restore, err = e2eutil.WaitForRestorePhase(t, restore, api.RestorePhaseComplete, restoreBackoff, f.MySQLOpClient)
	if err != nil {
		t.Fatalf("Restore failed to reach phase %q: %v", api.RestorePhaseComplete, err)
	}
	if restore.Labels[constants.MySQLOperatorVersionLabel] != f.BuildVersion {
		t.Errorf("Restore MySQLOperatorVersionLabel was incorrect: %s != %s.", restore.Labels[constants.MySQLOperatorVersionLabel], f.BuildVersion)
	} else {
		t.Logf("Restore label MySQLOperatorVersionLabel: %s", restore.Labels[constants.MySQLOperatorVersionLabel])
	}

	// ---------------------------------------------------------------------- //
	t.Log("trying connection to container")
	// ---------------------------------------------------------------------- //
	err = e2eutil.Retry(e2eutil.DefaultRetry, func() (bool, error) {
		passwd, err := testdb.GetPassword()
		return passwd != "", err
	})
	if err != nil {
		t.Fatalf("Failed to connect to the database")
	}

	// ---------------------------------------------------------------------- //
	t.Log("validating database...")
	// ---------------------------------------------------------------------- //
	testdb.Test()

	t.Report()
}
