package config

const (
	QueryDBInstanceRunningInfoSqlStr = `
        SELECT 
               TO_CHAR(START_TIME,'YYYY-MM-DD HH24:MI:SS'),
               CASE STATUS$ WHEN 'OPEN' THEN '1' WHEN 'MOUNT' THEN '2' WHEN 'SUSPEND' THEN '3' ELSE '4' END AS STATUS,
               CASE MODE$ WHEN 'PRIMARY' THEN '1' WHEN 'NORMAL' THEN '2' WHEN 'STANDBY' THEN '3' ELSE '4' END AS MODE,
               (SELECT COUNT(*) FROM V$TRXWAIT) TRXNUM,
               (SELECT COUNT(*) FROM V$LOCK WHERE BLOCKED=1) DEADLOCKNUM,
               (SELECT COUNT(*) FROM V$THREADS) THREADSNUM
        FROM V$INSTANCE`

	QueryTablespaceFileSqlStr = `SELECT PATH,
            TO_CHAR(TOTAL_SIZE*PAGE) AS TOTAL_SIZE,
            TO_CHAR(FREE_SIZE*PAGE)AS FREE_SIZE,
            AUTO_EXTEND,
            NEXT_SIZE,
            MAX_SIZE
    FROM V$DATAFILE;`

	QueryDBSessionsSqlStr = "SELECT COUNT(*) FROM v$sessions"
	// 其他SQL查询...
)
