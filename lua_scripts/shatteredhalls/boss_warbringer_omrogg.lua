-- Warbringer O'mrogg (Shattered Halls, Hellfire Citadel) — Lua
-- port of src/server/scripts/Outland/HellfireCitadel/
-- ShatteredHalls/boss_warbringer_omrogg.cpp (boss_warbringer_
-- omrogg — the boss AI class AddSC registers; npc_omrogg_
-- heads is registered alongside).
-- Second boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- cpp stays blocked on the instance-script model).
-- Entry: no NPC_OMROGG entry exists in shattered_halls.h and
-- instance_shattered_halls.cpp has no OnCreatureCreate
-- mapping for the boss, so the entry is verified externally:
-- wowhead npc=16809/warbringer-omrogg, mmo4ever creature
-- 16809. The two heads are C++ Creatures-enum constants in
-- the .cpp itself: NPC_LEFT_HEAD = 19523, NPC_RIGHT_HEAD =
-- 19524. The creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- Talk: the boss itself only ever talks EMOTE_ENRAGE = 2
-- (fired from the burning-maul arm, C++-exact); YELL_DIE_L
-- = 0 / YELL_DIE_R = 1 and the whole GoCombat/GoCombatDelay/
-- Threat/ThreatDelay1/ThreatDelay2/Killing/KillingDelay
-- talk tables are PORTED below as Talk() on the summoned-head
-- handles (stored at pull — no ObjectAccessor GUID relay needed)
-- plus the 3.5s Delay_Timer pump in C++ check order.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): O'mrogg
-- (16809): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize() values (blastWave 0 / blastCount 0 / fear
-- 8000 / burningMaul 25000 / thunderclap 15000 /
-- resetThreat 30000 — the C++ JustEngagedWith override summons
-- the heads, talks through the left head and pokes the instance
-- (summons + aggro yell PORTED below; only the instance arm
-- stays out), so the Initialize() schedule lands on the only
-- engagement hook this model has) + 1s scheduler pump (a port of
-- UpdateAI in C++ arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump).
-- Pump: blast wave 30600 — fires only while blastCount is
-- set: 16s after burning maul then 5s later (2 casts total,
-- C++-exact bookkeeping), non-triggered self-cast (C++ DoCast(me),
-- C++-exact — the old CastSpell(nil, ...) arm was DoCastVictim-
-- shaped in the Go engine: nil falls back to the combat victim;
-- retargeted this run, same class on the three self-cast arms
-- below); burning maul 30598: 25s init then 40s —
-- Talk(EMOTE_ENRAGE), non-triggered self-cast (C++ DoCast(me),
-- C++-exact), re-arm 40000, blastWave = 16000, blastCount = 1
-- (C++-exact); reset threat 30s init then {25s,40s} —
-- random threat-list pick + ResetAllThreat + AddThreat(pick, 0.0)
-- PORTED with its DoYellForThreat head-talk (the heads are
-- summoned at pull and their handles stored, so no GUID
-- relay is needed), re-arm
-- regardless (C++-exact); fear 30584: 8s init then {15s,35s}
-- — non-triggered self-cast (C++ DoCast(me), C++-exact),
-- re-arm {15s,35s} regardless (C++-exact); thunderclap
-- 30633: 15s init then {15s,30s} — non-triggered self-cast
-- (C++ DoCast(me), C++-exact), re-arm {15s,30s} regardless
-- (C++-exact).
-- OnDied(4): left-head Talk(YELL_DIE_L) + 4s right-head
-- Talk(YELL_DIE_R) + despawn (C++ JustDied/SetData arms,
-- PORTED below); the _JustDied arm is instance-blocked.
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop per-GUID
-- state, despawn both heads (C++ Reset setDeathState arms,
-- PORTED below); the _Reset arm is instance-blocked.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_shattered_halls.cpp stays blocked on the
-- instance-script model; the BossAI ctor DATA_OMROGG = 1
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped —
-- DATA_OMROGG is an instance-side constant, unbridgeable;
-- the JustEngagedWith SetBossState(DATA_OMROGG, IN_PROGRESS)
-- arm, the Reset SetData(DATA_OMROGG, NOT_STARTED) arm and
-- the DATA_KARGATH cross-boss assist arms in instance_
-- shattered_halls.cpp are instance gated); the JustEngagedWith
-- SummonCreature(NPC_LEFT_HEAD/NPC_RIGHT_HEAD,
-- TEMPSUMMON_DEAD_DESPAWN) calls + JustSummoned SetVisible
-- (false) + left-head GoCombat aggro yell are PORTED in the
-- OnEnterCombat handler (the GUID tracking itself is unneeded:
-- the Delay_Timer yell machine it feeds is PORTED below
-- (3.5s pump in C++ check order, head handles stored at pull,
-- no GUID relay needed); no ObjectAccessor/cross-creature
-- bridge is needed for it; npc_omrogg_heads documented only,
-- not registered — its SetData -> 4s EVENT_DEATH_YELL ->
-- Talk(YELL_DIE_R) + setDeathState arm is driven from the boss
-- JustDied handler below, and its Reset/JustEngagedWith
-- overrides are empty (ahune bunny / nethekurse fissure
-- precedent); the DoYellForThreat head-talk arm is PORTED
-- with its threat legs (random threat-list pick + ResetAllThreat
-- + AddThreat(pick, 0.0)); H_SPELL_BURNING_MAUL 36056 is an
-- unused C++ enum member (the cast is unconditional, C++-
-- exact); the ScriptData "Heroic enabled. Spell timing
-- may need additional tweaks" (SD%Complete: 85) is an
-- upstream caveat, documented, not bridged; no movement
-- bridge — no movement arms in this file; no SpellScript/
-- AuraScript scripts in this file.

local SPELL_BLAST_WAVE = 30600
local SPELL_FEAR = 30584
local SPELL_THUNDERCLAP = 30633
local SPELL_BURNING_MAUL = 30598

local EMOTE_ENRAGE = 2

local ENTRY_OMROGG = 16809

local NPC_LEFT_HEAD = 19523
local NPC_RIGHT_HEAD = 19524

local timers = {}
local states = {}
local delayTimers = {}
local deathTimers = {}

-- C++ talk tables (boss_warbringer_omrogg.cpp:105-141), 0-indexed
-- in C++ so Lua indexes +1: {talkId, head side}.
local THREAT = { {3, "L"}, {3, "R"}, {4, "L"}, {5, "L"} }
local THREAT_DELAY1 = { {4, "R"}, {6, "L"}, {5, "R"}, {6, "R"} }
local THREAT_DELAY2 = { {7, "L"}, {7, "R"}, {8, "L"}, {9, "L"} }
local KILLING = { {10, "L"}, {8, "R"} }
local KILLING_DELAY = { {9, "R"}, {11, "L"} }

local function headFor(st, side)
    if side == "L" then
        return st.leftHead
    end
    return st.rightHead
end

local function cancelDelayPump(guid)
    local id = delayTimers[guid]
    if id then
        RemoveEventById(id)
        delayTimers[guid] = nil
    end
end

local function despawnHeads(st)
    if st.leftHead then
        st.leftHead:Despawn()
    end
    if st.rightHead then
        st.rightHead:Despawn()
    end
end

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    cancelDelayPump(guid)
    states[guid] = nil
end

-- C++ Initialize() values (the JustEngagedWith head summons +
-- aggro yell are PORTED in the OnEnterCombat handler below;
-- only the instance SetBossState arm stays out, so the schedule
-- lands on OnEnterCombat):
-- blastWave 0, blastCount 0, fear 8000, burningMaul 25000,
-- thunderclap 15000, resetThreat 30000.
local function freshState()
    return {
        blastWave = 0,
        blastCount = 0,
        fear = 8000,
        burningMaul = 25000,
        thunderclap = 15000,
        resetThreat = 30000,
        leftHead = nil,
        rightHead = nil,
        iaggro = 0,
        ithreat = 0,
        ikilling = 0,
        aggroYell = false,
        threatYell = false,
        threatYell2 = false,
        killingYell = false,
    }
end

-- C++ UpdateAI Delay_Timer block (boss_warbringer_omrogg.cpp:293-330):
-- fires every 3.5s, checks in C++ order AggroYell, ThreatYell2,
-- ThreatYell, KillingYell. ThreatYell sets ThreatYell2 for the NEXT
-- firing (its check already passed this tick), so the threat chain
-- is first line at T, delay1 at T+3.5s, delay2 at T+7s. The block
-- returns early unless both heads exist (C++-exact gate).
local function delayTick(creature, guid)
    local st = states[guid]
    if not st or not st.leftHead or not st.rightHead then
        return
    end
    if st.aggroYell then
        -- GoCombatDelay is always the right head, id = iaggro.
        st.rightHead:Talk(st.iaggro)
        st.aggroYell = false
    end
    if st.threatYell2 then
        local leg = THREAT_DELAY2[st.ithreat + 1]
        local head = headFor(st, leg[2])
        if head then
            head:Talk(leg[1])
        end
        st.threatYell2 = false
    end
    if st.threatYell then
        local leg = THREAT_DELAY1[st.ithreat + 1]
        local head = headFor(st, leg[2])
        if head then
            head:Talk(leg[1])
        end
        st.threatYell = false
        st.threatYell2 = true
    end
    if st.killingYell then
        local leg = KILLING_DELAY[st.ikilling + 1]
        local head = headFor(st, leg[2])
        if head then
            head:Talk(leg[1])
        end
        st.killingYell = false
    end
end

-- C++ Delay_Timer is one shared timer: every flag-set re-arms it to
-- 3.5s (JustEngagedWith, DoYellForThreat, KilledUnit case 0), so
-- re-arming the pump on each set is C++-exact, not just periodic.
local function armDelayPump(creature, guid)
    cancelDelayPump(guid)
    delayTimers[guid] = CreateLuaEvent(function()
        delayTick(creature, guid)
    end, 3500, 0)
end

-- C++ SelectTarget(Random, 0) draws from the threat list
-- (UnitAI.h:170, playerOnly defaults false) — GetAITargets is
-- the bridged threat list (murmur/thaurissan precedent), not
-- the instance player pool.
local function randomThreatTarget(creature)
    local alive = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Blast wave 30600: fires only while blastCount is set —
    -- 16s after burning maul then 5s later, 2 casts total
    -- (C++-exact bookkeeping), non-triggered self-cast
    -- (C++ DoCast(me) — the nil arm fell back to the victim).
    if st.blastCount > 0 then
        if st.blastWave <= 1000 then
            creature:CastSpell(creature, SPELL_BLAST_WAVE)
            st.blastCount = st.blastCount + 1
            if st.blastCount == 3 then
                st.blastCount = 0
            else
                st.blastWave = 5000
            end
        else
            st.blastWave = st.blastWave - 1000
        end
    end

    -- Burning maul 30598: 25s init then 40s — Talk(EMOTE_
    -- ENRAGE), non-triggered self-cast (C++ DoCast(me), C++-exact),
    -- re-arm
    -- 40000, blastWave = 16000, blastCount = 1 (C++-exact).
    -- (H_SPELL 36056 is an unused C++ enum member.)
    if st.burningMaul <= 1000 then
        creature:Talk(EMOTE_ENRAGE)
        creature:CastSpell(creature, SPELL_BURNING_MAUL)
        st.burningMaul = 40000
        st.blastWave = 16000
        st.blastCount = 1
    else
        st.burningMaul = st.burningMaul - 1000
    end

    -- Reset threat: 30s init then {25s,40s} — random
    -- threat-list pick (C++ SelectTarget Random, 0, playerOnly
    -- defaults false) + ResetAllThreat + AddThreat(pick, 0.0) in C++
    -- order; nil pick leaves the list untouched (C++ gates the
    -- whole block on SelectTarget non-nil); the DoYellForThreat
    -- head-talk is PORTED above (DoYellForThreat leg). Re-arm
    -- regardless (C++-exact).
    if st.resetThreat <= 1000 then
        local pick = randomThreatTarget(creature)
        if pick ~= nil then
            -- C++ DoYellForThreat() before ResetThreatList/AddThreat
            -- (boss_warbringer_omrogg.cpp:379-384): first threat line
            -- from the table head; the delay pump delivers delay1/
            -- delay2. Both-heads-gated like C++ (DoYellForThreat
            -- returns early without heads, threat legs still run).
            if st.leftHead and st.rightHead then
                st.ithreat = math.random(0, 3)
                local leg = THREAT[st.ithreat + 1]
                local head = headFor(st, leg[2])
                if head then
                    head:Talk(leg[1])
                end
                st.threatYell = true
                armDelayPump(creature, guid)
            end
            creature:ResetAllThreat()
            creature:AddThreat(pick, 0.0)
        end
        st.resetThreat = math.random(25000, 39999)
    else
        st.resetThreat = st.resetThreat - 1000
    end

    -- Fear 30584: 8s init then {15s,35s} — non-triggered
    -- self-cast (C++ DoCast(me), C++-exact), re-arm
    -- regardless (C++-exact).
    if st.fear <= 1000 then
        creature:CastSpell(creature, SPELL_FEAR)
        st.fear = math.random(15000, 34999)
    else
        st.fear = st.fear - 1000
    end

    -- Thunderclap 30633: 15s init then {15s,30s} —
    -- non-triggered self-cast (C++ DoCast(me), C++-exact),
    -- re-arm regardless
    -- (C++-exact).
    if st.thunderclap <= 1000 then
        creature:CastSpell(creature, SPELL_THUNDERCLAP)
        st.thunderclap = math.random(15000, 29999)
    else
        st.thunderclap = st.thunderclap - 1000
    end
end

RegisterCreatureEvent(ENTRY_OMROGG, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    local st = freshState()
    states[guid] = st
    -- C++ JustEngagedWith (boss_warbringer_omrogg.cpp:202-213):
    -- SummonCreature both heads at the boss (all-zero coords hit
    -- the bridge's near-summoner fallback), JustSummoned
    -- SetVisible(false) on each, then the left head talks a random
    -- GoCombat line (ids 0-2, all LEFT_HEAD) with the 3.5s
    -- Delay_Timer follow-up armed; the instance SetBossState arm
    -- stays out.
    local leftHead = creature:SummonCreature(NPC_LEFT_HEAD, 0, 0, 0, 0, 6)
    if leftHead then
        leftHead:SetVisible(false)
    end
    local rightHead = creature:SummonCreature(NPC_RIGHT_HEAD, 0, 0, 0, 0, 6)
    if rightHead then
        rightHead:SetVisible(false)
    end
    st.leftHead = leftHead
    st.rightHead = rightHead
    if leftHead then
        st.iaggro = math.random(0, 2)
        leftHead:Talk(st.iaggro)
        st.aggroYell = true
        armDelayPump(creature, guid)
    end
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

-- C++ KilledUnit (boss_warbringer_omrogg.cpp:234-256): both heads
-- must exist; immediate first line from the table head, case 0
-- arms the 3.5s KillingDelay follow-up, case 1 clears it. No
-- player gate in C++ (C++-exact: event 3 fires on any kill).
RegisterCreatureEvent(ENTRY_OMROGG, 3, function(_, creature)
    local guid = creature:GetGUID()
    local st = states[guid]
    if not st or not st.leftHead or not st.rightHead then
        return
    end
    st.ikilling = math.random(0, 1)
    local leg = KILLING[st.ikilling + 1]
    local head = headFor(st, leg[2])
    if head then
        head:Talk(leg[1])
    end
    if st.ikilling == 0 then
        st.killingYell = true
        armDelayPump(creature, guid)
    else
        st.killingYell = false
    end
end)

RegisterCreatureEvent(ENTRY_OMROGG, 2, function(_, creature)
    -- C++ Reset kills both heads (setDeathState JUST_DIED).
    local st = states[creature:GetGUID()]
    if st then
        despawnHeads(st)
    end
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_OMROGG, 4, function(_, creature)
    -- C++ JustDied (boss_warbringer_omrogg.cpp:262-273): both heads
    -- must exist; left head talks YELL_DIE_L at once, right head
    -- talks YELL_DIE_R 4s later (SetData arm) then dies. The delayed
    -- leg captures the head handle directly, so it survives the
    -- state clear below; the left head lingers, as in C++.
    local guid = creature:GetGUID()
    local st = states[guid]
    if st and st.leftHead and st.rightHead then
        st.leftHead:Talk(0)
        local rightHead = st.rightHead
        deathTimers[guid] = CreateLuaEvent(function()
            rightHead:Talk(1)
            rightHead:Despawn()
            deathTimers[guid] = nil
        end, 4000)
    end
    resetBoss(guid)
end)

RegisterCreatureEvent(ENTRY_OMROGG, 23, function(_, creature)
    -- Event 23 fires on death too (engine reset-then-died order):
    -- pumps stop either way, but heads are only despawned on a
    -- live reset — a death-time reset must leave them for event 4,
    -- so state is preserved when dead and cleared by event 4.
    local guid = creature:GetGUID()
    cancelPump(guid)
    cancelDelayPump(guid)
    if creature:IsAlive() then
        local st = states[guid]
        if st then
            despawnHeads(st)
        end
        states[guid] = nil
    end
end)
