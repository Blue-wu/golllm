import time
from pymilvus import connections, utility

print("Starting Milvus Lite...")
connections.connect(host='localhost', port=19530)
print("Milvus Lite is ready!")
print(f"Connected: {utility.get_server_version()}")

while True:
    time.sleep(3600)
