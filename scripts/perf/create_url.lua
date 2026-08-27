-- wrk Lua 脚本用于测试创建短链接的性能

-- 请求计数器
request_counter = 0

-- 生成请求体
request = function()
    request_counter = request_counter + 1
    
    -- 生成唯一的 URL
    local long_url = string.format("https://perf-test-%d-%d.com", 
                                   os.time(), 
                                   request_counter)
    
    local body = string.format(
        '{"long_url":"%s","user_id":"wrk_perf_test"}',
        long_url
    )
    
    return wrk.format("POST", "/api/v1/shorten", 
                     {["Content-Type"] = "application/json"}, 
                     body)
end

-- 响应处理
response = function(status, headers, body)
    if status ~= 200 and status ~= 201 then
        print("Error response: " .. status)
        print("Body: " .. body)
    end
end

-- 测试完成后的统计
done = function(summary, latency, requests)
    io.write("------------------------------\n")
    io.write("测试总结:\n")
    io.write(string.format("  总请求数: %d\n", summary.requests))
    io.write(string.format("  总耗时: %.2f 秒\n", summary.duration / 1000000))
    io.write(string.format("  QPS: %.2f\n", summary.requests / (summary.duration / 1000000)))
    io.write(string.format("  平均延迟: %.2f ms\n", latency.mean / 1000))
    io.write(string.format("  最小延迟: %.2f ms\n", latency.min / 1000))
    io.write(string.format("  最大延迟: %.2f ms\n", latency.max / 1000))
    io.write(string.format("  P50 延迟: %.2f ms\n", latency:percentile(50) / 1000))
    io.write(string.format("  P90 延迟: %.2f ms\n", latency:percentile(90) / 1000))
    io.write(string.format("  P99 延迟: %.2f ms\n", latency:percentile(99) / 1000))
    io.write("------------------------------\n")
end
