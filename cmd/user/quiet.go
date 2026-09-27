package main

// quiet — audit · verify 처럼 여러 단계를 한 명령으로 묶은 것이 raRequest · raParse · register · lookup 을 부를 때 켠다.
// 그러면 그 함수들은 자기 출력을 내지 않고, 부른 쪽이 "지금 무엇을 하는지" 한 줄씩 찍는다.
var quiet bool
