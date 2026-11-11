# [Under Construction] v0.1 Result + Session System

# 我需要什麼功能？

- Session file 儲存，來源可能是 runner 也可以是我所有 endpoint 上 pi cli agent 的 session ( 上傳 command 已經寫到 extension 中了 )
- Session 要有 type
    - Pi claude
    - gemini
    - … future type ?
- Session Upload 必須要能重複上傳，第一版先覆蓋，後續再決定需不需要做版本紀錄
- Session References: 必須要 global unique ( maybe a uuid ? session id + unix time ? like mongodb object id)
    - Q: type + uuid maybe ?
- Tool Call ID in Session References → 這個已經實作 tool call ref extension 可以參考 pi agent folder
    - 這個是所有 report generation 的核心，非常重要。必須要有辦法讓 model 在最小 token 消耗的方式去取得 tool call 的 result
- Tool Call Result 也要上傳 !
    - pi agent 會把一些太長的東西送到 tmp file, 這個東西在 session upload 的時候要一起上傳
        - 展開 .. ? or separate ?
        - Q: codemode 還會受這影響嗎？
- Report System ( 儲存 )
    - 會有一個固定的 json format
    - 不要讓 agent 自己 gen html report, 非常耗 token 也不好管理
- Reference system
    - 本質上 Result System 背後會有個 blob storage, 我們需要把 reference 存進來 for reference
    - 定義幾種 references
        - opensource 專案 ( github 永久連結 + 檔案行數 + 短數量 code 擷取 )
        - document 頁面 ( 網址 + 文字擷取 )
        - 純粹一般文字擷取 ( 網址 + 文字擷取 )
        - [custom] synology / workplus issue 等等 ( 這個會放在 project instruction 裡面 )
        - … TBD ?
- Report / Session 管理頁面 ( Frontend 顯示, 我不覺得這個 case 需要開 vue 等專案來處理。他就是個 dashboard)
    - Search History Session ( Text Search )
    - 前端頁面 render ( 這就是我們可以處理的 UI rendering 了 )
    - 可以參考一下 pi 原生 export 功能
- Report Compact Form for Agents
    - 同個 json report 要可以 render 成 compact form 這樣其他 agent 才可以正確讀取

# 實作上需要先考量的東西

## 系統中有哪些物件？哪些是 first class object

- Report ( 包含各個不同版本的 Report )
- References Object ( 單一 version, 不可改動 )
- Session ( 所有上傳的 session )

## Relation 關聯性

- Report 可以連接到多個 Session 和多個 References Objects
- Relationship, 這個 edge 要有狀態
    - `pending`, `linked`
    - Case: Report 有可能提前 session 上傳，link 要顯示 pending

## Report Versioning

- v1 → v2 → v3, 有修改就要加一版
- 有可能是不同 sessions 一直修改的內容
- 上傳要有 lock, 防止 race condition.
    - what about conflict ? [ TBD ]

## Report Format

- 需要有 version, 確保 render module 可以改掉

## Reference 結構

- 資料要直接抓下來不要只存連結
- 可以存個 excerpt ( 用 haiku 什麼的去產 description ), 但這東西需要檢查，不能直接相信 ( search and check )

## Retention Policy

尤其是 reference 可不可以刪除？需要有辦法 trace 出這個 reference ( 包含 tool call 是否還正在被 referenced )

- 有沒有其他 object 正 reference 到這個 node, 這需要檢查
- 任何 reference 到 tool calling 的行為，代表 session node 不能被 retention 掉

## Blob 儲存方式

- 用 Hash 來當 path, 可以簡單當作 deduplication ( sha256 content hash )

## Session 本身的特性

- Compact, Branches 這要研究一下
- Pi session tools result 會需要上傳，在太長的 bash call 的狀態下 session file 中只會儲存 truncated version, 要連帶把結果一起上傳
    - 我覺得可能要改變 path 放法 ( tmp/{uuid}.txt ), 這樣可以確保我用 file path per session 可以正確的 reference 到對的檔案

## Search 功能 [待討論]

- 中文要能有辦法搜尋，或是說 excerpt 強制翻成英文
- What to index ( DB 層級 )
- inactive branches in session file should be marked in search result.