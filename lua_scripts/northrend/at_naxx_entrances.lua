-- Naxxramas entrance area triggers — Lua port of the
-- AreaTriggerScript classes in
-- src/server/scripts/Northrend/Naxxramas/boss_anubrekhan.cpp:243-261,
-- boss_faerlina.cpp:267-285, boss_kelthuzad.cpp:984-1005 and
-- boss_thaddius.cpp:1217-1235. Triggers 4119 / 4115 / 4112 / 4113
-- (TDB 335.26091 areatrigger_scripts rows). Fire site:
-- engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the sartura / at_scripts.lua precedent). The handler return
-- is discarded, so the C++ `return true` legs are moot here.
-- PORTED 2026-11-15 (at_* census). The bosses read through the
-- instance_naxxramas.lua binds (data.anubrekhan / .faerlina /
-- .thaddius / .kelthuzad); creature:GetBossState reads the shared
-- (map, instance) store (lua_creature_events.go:7559), treating
-- TO_BE_DECIDED (5, never set) as NOT_STARTED like C++ does
-- (the at_hor_triggers.lua precedent — the boss files write
-- NOT_STARTED on Reset, so the read is normally 0). The
-- OnlyOnce latches ride the instanceData table (the
-- nerubar-broodkeeper precedent: latch first — C++ marks the
-- trigger done on first fire even when the Talk is gated off —
-- then gate; the latch does not clear on instance reset, where
-- C++ ResetAreaTriggerDone would re-arm the greeting —
-- negligible, noted). Kelthuzad's receiver is PORTED
-- (kelthuzadDoAction ev38, boss_kelthuzad.lua) and re-gates
-- NOT_STARTED / TO_BE_DECIDED itself.

local TRIGGER_ANUBREKHAN_ENTRANCE = 4119 -- TDB areatrigger_scripts row
local TRIGGER_FAERLINA_ENTRANCE = 4115 -- TDB areatrigger_scripts row
local TRIGGER_KELTHUZAD_CENTER = 4112 -- TDB areatrigger_scripts row
local TRIGGER_THADDIUS_ENTRANCE = 4113 -- TDB areatrigger_scripts row

local BOSS_ANUBREKHAN = 0 -- naxxramas.h:30
local BOSS_FAERLINA = 1 -- naxxramas.h:31
local BOSS_THADDIUS = 9 -- naxxramas.h:39
local BOSS_KELTHUZAD = 14 -- naxxramas.h:44

local ACTION_BEGIN_ENCOUNTER = 0 -- boss_kelthuzad.cpp:74
local SAY_GREET_ANUBREKHAN = 1 -- boss_anubrekhan.cpp:28
local SAY_GREET_FAERLINA = 0 -- boss_faerlina.cpp:28
local SAY_GREET_THADDIUS = 0 -- boss_thaddius.cpp:129

local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_DONE = 3
local BOSS_STATE_TO_BE_DECIDED = 5 -- Go unset-boss read

RegisterServerEvent(24, function(event, player, triggerId)
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    if triggerId == TRIGGER_ANUBREKHAN_ENTRANCE then
        -- C++ :250-259 at_anubrekhan_entrance::TryHandleOnce
        -- (OnlyOnce: latch first).
        if data.atAnubrekhanDone then
            return
        end
        data.atAnubrekhanDone = true
        local anub = data.anubrekhan
        if anub then
            local state = anub:GetBossState(BOSS_ANUBREKHAN)
            if state == BOSS_STATE_NOT_STARTED or state == BOSS_STATE_TO_BE_DECIDED then
                anub:Talk(SAY_GREET_ANUBREKHAN)
            end
        end
    elseif triggerId == TRIGGER_FAERLINA_ENTRANCE then
        -- C++ :274-283 at_faerlina_entrance::TryHandleOnce
        -- (OnlyOnce: latch first).
        if data.atFaerlinaDone then
            return
        end
        data.atFaerlinaDone = true
        local faerlina = data.faerlina
        if faerlina then
            local state = faerlina:GetBossState(BOSS_FAERLINA)
            if state == BOSS_STATE_NOT_STARTED or state == BOSS_STATE_TO_BE_DECIDED then
                faerlina:Talk(SAY_GREET_FAERLINA)
            end
        end
    elseif triggerId == TRIGGER_KELTHUZAD_CENTER then
        -- C++ :989-1003 at_kelthuzad_center::OnTrigger (plain:
        -- no latch). C++ order: boss-state gate, then GM gate,
        -- then DoAction.
        local kt = data.kelthuzad
        if kt then
            local state = kt:GetBossState(BOSS_KELTHUZAD)
            if (state == BOSS_STATE_NOT_STARTED or state == BOSS_STATE_TO_BE_DECIDED)
                and not player:IsGM() then
                kt:DoAction(ACTION_BEGIN_ENCOUNTER)
            end
        end
    elseif triggerId == TRIGGER_THADDIUS_ENTRANCE then
        -- C++ :1224-1233 at_thaddius_entrance::TryHandleOnce
        -- (OnlyOnce: latch first). Note the gate differs from
        -- the other three: != DONE, not NOT_STARTED.
        if data.atThaddiusDone then
            return
        end
        data.atThaddiusDone = true
        local thad = data.thaddius
        if thad and thad:GetBossState(BOSS_THADDIUS) ~= BOSS_STATE_DONE then
            thad:Talk(SAY_GREET_THADDIUS)
        end
    end
end)
