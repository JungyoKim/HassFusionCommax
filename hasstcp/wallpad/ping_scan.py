import subprocess
import platform
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed

def ping_host(ip):
    """
    단일 IP 주소에 ping을 보내고 결과를 반환합니다.
    """
    try:
        # 운영체제에 따라 ping 명령어 결정
        if platform.system().lower() == "windows":
            cmd = ["ping", "-n", "1", "-w", "1000", ip]
        else:
            cmd = ["ping", "-c", "1", "-W", "1", ip]
        
        # ping 실행
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=2)
        
        if result.returncode == 0:
            return ip, True, "응답함"
        else:
            return ip, False, "응답 없음"
            
    except subprocess.TimeoutExpired:
        return ip, False, "시간 초과"
    except Exception as e:
        return ip, False, f"오류: {str(e)}"

def scan_network():
    """
    10.9.1.0부터 10.9.1.255까지 ping 스캔을 수행합니다.
    """
    base_ip = "10.9.0"
    active_hosts = []
    inactive_hosts = []
    
    print(f"{base_ip}.0부터 {base_ip}.255까지 ping 스캔을 시작합니다...")
    print("=" * 50)
    
    # ThreadPoolExecutor를 사용하여 병렬 처리
    with ThreadPoolExecutor(max_workers=50) as executor:
        # 모든 IP에 대한 ping 작업 제출
        future_to_ip = {
            executor.submit(ping_host, f"{base_ip}.{i}"): f"{base_ip}.{i}" 
            for i in range(256)
        }
        
        # 결과 수집
        for future in as_completed(future_to_ip):
            ip, is_active, message = future.result()
            
            if is_active:
                active_hosts.append(ip)
                print(f"✓ {ip:<15} - {message}")
            else:
                inactive_hosts.append(ip)
                print(f"✗ {ip:<15} - {message}")
    
    # 결과 요약
    print("\n" + "=" * 50)
    print("스캔 완료!")
    print(f"활성 호스트: {len(active_hosts)}개")
    print(f"비활성 호스트: {len(inactive_hosts)}개")
    
    if active_hosts:
        print("\n활성 호스트 목록:")
        for host in sorted(active_hosts, key=lambda x: int(x.split('.')[-1])):
            print(f"  - {host}")
    
    return active_hosts, inactive_hosts

if __name__ == "__main__":
    try:
        start_time = time.time()
        active, inactive = scan_network()
        end_time = time.time()
        
        print(f"\n스캔 소요 시간: {end_time - start_time:.2f}초")
        
    except KeyboardInterrupt:
        print("\n\n사용자에 의해 중단되었습니다.")
    except Exception as e:
        print(f"\n오류가 발생했습니다: {e}")
