from airflow import DAG
from airflow.providers.postgres.hooks.postgres import PostgresHook
from clickhouse_driver import Client
from airflow.operators.python import PythonOperator
from airflow.models import Connection
from airflow.utils.session import provide_session
from datetime import datetime, timedelta
import pandas as pd

PG_HOST = "bionic-postgres-demo"
PG_PORT = 5432
PG_DB = "demo_db"
PG_USER = "demo_user"
PG_PASS = "demo_pass"

CH_HOST = "clickhouse"
CH_PORT = 9000
CH_DB = "bionic_analytics"
CH_USER = "default"

WINDOW_HOURS = 24
CONN_ID = "pg_inline_conn"

@provide_session
def ensure_postgres_connection(session=None):
    try:
        PostgresHook.get_connection(CONN_ID)
    except Exception:
        conn = Connection(
            conn_id=CONN_ID,
            conn_type='postgres',
            host=PG_HOST,
            login=PG_USER,
            password=PG_PASS,
            schema=PG_DB,
            port=PG_PORT
        )
        conn.set_extra('')
        session.add(conn)
        session.commit()

def build_report_vitrina(**context):
    ensure_postgres_connection()
    pg_hook = PostgresHook(postgres_conn_id=CONN_ID)
    conn = pg_hook.get_conn()
    cutoff_time = datetime.utcnow() - timedelta(hours=WINDOW_HOURS)

    query = """
        SELECT u.login, t.device_id, t.event_time, t.emg_signal_mean,
               t.command_recognized, t.confidence_score,
               t.battery_level_pct, t.battery_temp_c,
               t.movement_detected, t.error_code
        FROM telemetry_raw t
        JOIN crm_users u ON t.device_id = u.device_id
        WHERE t.event_time >= %s
        ORDER BY t.event_time
    """
    
    df = pd.read_sql(query, conn, params=(cutoff_time,))
    conn.close()

    if df.empty:
        print("Нет данных за последние {} часов".format(WINDOW_HOURS))
        return

    df["event_time"] = pd.to_datetime(df["event_time"])
    df["hour_bucket"] = df["event_time"].dt.floor("H")

    agg_df = df.groupby(["login", "device_id", "hour_bucket"]).agg(
        avg_emg=("emg_signal_mean", "mean"),
        avg_confidence=("confidence_score", "mean"),
        avg_battery_lvl=("battery_level_pct", "mean"),
        avg_battery_temp=("battery_temp_c", "mean"),
        grasp_count=("command_recognized", lambda x: (x == "GRASP").sum()),
        rotate_count=("command_recognized", lambda x: (x == "ROTATE").sum()),
        total_events=("event_time", "count"),
        error_count=("error_code", lambda x: x.notnull().sum())
    ).reset_index()

    ch_client = Client(host=CH_HOST, port=CH_PORT, database=CH_DB, user=CH_USER)

    create_table_query = """
        CREATE TABLE IF NOT EXISTS user_telemetry_hourly (
            user_login String,
            device_id String,
            hour_bucket DateTime,
            avg_emg Float64,
            avg_confidence Float64,
            avg_battery_lvl Float64,
            avg_battery_temp Float64,
            grasp_count UInt32,
            rotate_count UInt32,
            total_events UInt32,
            error_count UInt32
        ) ENGINE = ReplacingMergeTree()
        ORDER BY (user_login, device_id, hour_bucket)
    """
    ch_client.execute(create_table_query)

    min_time = agg_df["hour_bucket"].min()
    max_time = agg_df["hour_bucket"].max()

    existing_keys_query = """
        SELECT user_login, device_id, hour_bucket
        FROM user_telemetry_hourly
        WHERE hour_bucket BETWEEN %(min_time)s AND %(max_time)s
    """
    existing_rows = ch_client.execute(existing_keys_query, {"min_time": min_time, "max_time": max_time})
    existing_set = set(existing_rows)

    def is_new(row):
        key = (row["login"], row["device_id"], row["hour_bucket"])
        return key not in existing_set

    new_agg_df = agg_df[agg_df.apply(is_new, axis=1)]

    if new_agg_df.empty:
        print("Новые данные для загрузки отсутствуют.")
        return

    rows = list(zip(
        new_agg_df["login"],
        new_agg_df["device_id"],
        new_agg_df["hour_bucket"],
        new_agg_df["avg_emg"],
        new_agg_df["avg_confidence"],
        new_agg_df["avg_battery_lvl"],
        new_agg_df["avg_battery_temp"],
        new_agg_df["grasp_count"],
        new_agg_df["rotate_count"],
        new_agg_df["total_events"],
        new_agg_df["error_count"]
    ))

    insert_query = """
        INSERT INTO user_telemetry_hourly (
            user_login, device_id, hour_bucket,
            avg_emg, avg_confidence, avg_battery_lvl,
            avg_battery_temp, grasp_count, rotate_count,
            total_events, error_count
        ) VALUES
    """
    
    ch_client.execute(insert_query, params=rows)
    print(f"Загружено новых строк: {len(rows)}")

with DAG(
    dag_id="build_report",
    schedule_interval="*/5 * * * *",
    start_date=datetime(2024, 1, 1),
    catchup=False,
    tags=["etl", "telemetry", "report"]
) as dag:

    task_build_report = PythonOperator(
        task_id="build_report_vitrina",
        python_callable=build_report_vitrina
    )

    task_build_report
