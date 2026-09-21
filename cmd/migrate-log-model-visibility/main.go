/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var (
	apply     = flag.Bool("apply", false, "rewrite legacy log metadata; omit for a dry run")
	batchSize = flag.Int("batch-size", 500, "number of logs to process in one transaction")
)

func main() {
	common.InitEnv()

	if *batchSize <= 0 {
		log.Fatal("batch-size must be positive")
	}
	if err := model.InitLogDBForMaintenance(); err != nil {
		log.Fatalf("open log database: %v", err)
	}
	defer func() {
		if err := model.CloseLogDBForMaintenance(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	report, err := model.MigrateLogModelVisibility(*apply, *batchSize)
	fmt.Printf("apply=%t scanned=%d candidates=%d migrated=%d remaining=%d\n", *apply, report.Scanned, report.Candidates, report.Migrated, report.Remaining)
	if err != nil {
		log.Fatalf("migrate legacy model diagnostics: %v", err)
	}
	if *apply && report.Remaining != 0 {
		log.Fatalf("migration incomplete: %d legacy logs remain", report.Remaining)
	}
}
