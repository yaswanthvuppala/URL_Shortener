from fastapi import FastAPI
app = FastAPI()

@app.get("/{code}")
def get_code(code: str):
    return {"code": code}

if __name__ == "__main__":
    import uvicorn
    import threading
    import requests
    import time
    
    def run_server():
        uvicorn.run(app, host="127.0.0.1", port=8005, log_level="critical")
        
    t = threading.Thread(target=run_server, daemon=True)
    t.start()
    time.sleep(1)
    
    res = requests.get("http://127.0.0.1:8005/docs")
    print("DOCS STATUS:", res.status_code)
    
    res2 = requests.get("http://127.0.0.1:8005/something")
    print("SOMETHING STATUS:", res2.status_code)
