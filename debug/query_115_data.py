# -*- coding: utf-8 -*-
"""
查询数据库中的115数据源和账号信息
用于Playwright测试准备
"""
import psycopg2
import json

# 数据库连接配置
DB_CONFIG = {
    'host': '192.168.31.12',
    'port': 15432,
    'database': 'easy_strm',
    'user': 'joshua',
    'password': 'dwwzbuwioalvb'
}

def query_115_accounts():
    """查询所有115账号"""
    conn = None
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        query = """
        SELECT id, name, cookie_status, created_at, updated_at
        FROM pan_accounts
        WHERE pan_type = '115'
        ORDER BY created_at DESC
        """

        cursor.execute(query)
        rows = cursor.fetchall()

        accounts = []
        for row in rows:
            accounts.append({
                'id': row[0],
                'name': row[1],
                'cookie_status': row[2],
                'created_at': str(row[3]),
                'updated_at': str(row[4])
            })

        return accounts

    except Exception as e:
        print(f"查询115账号失败: {e}")
        return []
    finally:
        if conn:
            conn.close()

def query_115_media_sources():
    """查询所有115数据源"""
    conn = None
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        query = """
        SELECT id, name, pan_account_id, root_folder_id, description, status, created_at, updated_at
        FROM media_sources
        WHERE pan_type = '115'
        ORDER BY created_at DESC
        """

        cursor.execute(query)
        rows = cursor.fetchall()

        sources = []
        for row in rows:
            sources.append({
                'id': row[0],
                'name': row[1],
                'pan_account_id': row[2],
                'root_folder_id': row[3],
                'description': row[4],
                'status': row[5],
                'created_at': str(row[6]),
                'updated_at': str(row[7])
            })

        return sources

    except Exception as e:
        print(f"查询115数据源失败: {e}")
        return []
    finally:
        if conn:
            conn.close()

if __name__ == '__main__':
    print("=" * 60)
    print("查询数据库中的115账号和数据源信息")
    print("=" * 60)

    # 查询115账号
    print("\n【115账号列表】")
    accounts = query_115_accounts()
    if accounts:
        for acc in accounts:
            print(f"  ID: {acc['id']}, 名称: {acc['name']}, 状态: {acc['cookie_status']}")
    else:
        print("  未找到115账号")

    # 查询115数据源
    print("\n【115数据源列表】")
    sources = query_115_media_sources()
    if sources:
        for src in sources:
            print(f"  ID: {src['id']}, 名称: {src['name']}, 账号ID: {src['pan_account_id']}, 根目录: {src['root_folder_id']}, 状态: {src['status']}")
    else:
        print("  未找到115数据源")

    # 保存到JSON文件供后续使用
    data = {
        'accounts': accounts,
        'sources': sources
    }

    with open('debug/test_data_115.json', 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

    print(f"\n数据已保存到: debug/test_data_115.json")
    print("=" * 60)
