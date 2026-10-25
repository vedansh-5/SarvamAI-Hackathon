import os

import pymongo

from brand import APP_NAME, DB_NAME

_db = None


def db():
    global _db
    if _db is None:
        uri = os.environ.get("MONGO_URI", "mongodb://localhost:27017")
        try:
            conn = pymongo.MongoClient(uri, serverSelectionTimeoutMS=800)
            conn.admin.command("ping")
            print(f"{APP_NAME}: MongoDB at {uri}")
        except Exception:
            import mongomock

            conn = mongomock.MongoClient()
            print(f"{APP_NAME}: MongoDB not reachable, keeping data in memory")
        _db = conn[os.environ.get("MONGO_DB", DB_NAME)]
    return _db
