def solve():
    # 입력 받기
    sleep_time, current_temp = map(int, input().split())
    target_temp = int(input())
    decrease, increase = map(int, input().split())
    
    boiler_time = 0
    temp = current_temp
    
    # 패턴 감지를 위한 상태 저장
    seen_states = {}
    hour = 0
    
    while hour < sleep_time:
        # 현재 상태 (온도, 보일러 상태)
        boiler_on = temp < target_temp
        state = (temp, boiler_on)
        
        # 사이클 감지
        if state in seen_states:
            # 사이클 발견
            cycle_start = seen_states[state]
            cycle_length = hour - cycle_start[0]
            cycle_boiler_time = boiler_time - cycle_start[1]
            
            # 남은 시간에서 완전한 사이클 개수 계산
            remaining_time = sleep_time - hour
            full_cycles = remaining_time // cycle_length
            
            # 완전한 사이클들의 보일러 시간 추가
            boiler_time += full_cycles * cycle_boiler_time
            hour += full_cycles * cycle_length
            
            # 사이클 감지 초기화 (나머지 시간 처리)
            seen_states.clear()
        
        # 상태 저장
        seen_states[state] = (hour, boiler_time)
        
        # 시뮬레이션
        if hour < sleep_time:
            if temp < target_temp:
                # 보일러 켜기
                boiler_time += 1
                temp += increase
            else:
                # 보일러 끄기
                temp += decrease
            
            hour += 1
    
    print(boiler_time)

solve()