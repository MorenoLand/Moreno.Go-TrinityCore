-- Lord Victor Nefarius (10162) and Nefarian (11583) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_nefarian.cpp (boss_victor_nefariusAI + boss_nefarianAI);
-- blackwing_lair.h:38 (DATA_NEFARIAN = 7, eighth boss), :60
-- (NPC_VICTOR_NEFARIUS = 10162), :61 (NPC_NEFARIAN = 11583).
-- C++ ScriptNames "boss_victor_nefarius" / "boss_nefarian" per
-- AddSC_boss_nefarian.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied (KilledUnit arm), 4 OnDied, 9 OnDamageTaken
-- (pre-damage hook, moroes convention), 23 OnReset. Eluna gossip
-- events: 1 OnGossipHello, 2 OnGossipSelect. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Victor Nefarius (C++-exact for the modeled arms): Reset arms
-- SpawnedAdds = 0 (Initialize). Gossip select on menu 6045 / option
-- 0: close the menu, Talk(SAY_GAMESBEGIN_1=12), BeginEvent ->
-- Talk(SAY_GAMESBEGIN_2=13), arm shadowbolt {3s,10s} then {3s,10s}
-- (50/50: shadowbolt volley 22665 non-triggered DoCastVictim, or
-- shadowbolt 22677 non-triggered on a random alive player in the
-- instance — C++ DoCast default is triggered=false, vaelastrasz
-- convention) / fear 22678 {10s,20s} then {10s,20s} non-triggered
-- on a random alive player (C++ SelectTarget Random 40 yd; shared
-- arena, the range check skipped) / spawn_add 10s then 4s.
-- EVENT_SPAWN_ADD: SpawnedAdds grows by 2 per wave (C++ sums two
-- drakonids per 4s firing); at >= 42 the C++ summons Nefarian and
-- drops Nefarius out of the fight — modeled as: cancel all timers,
-- mark the event done (the summons, the Nefarian summon and the
-- SetVisible(false) arm have no bridges, see deviations). The adds
-- themselves never spawn — only the 4s re-arm/count cycle is kept
-- (jeklik six-bat convention).
-- Nefarian (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_RANDOM=0), arm shadowflame 22539 12s then 12s /
-- bellowing roar 22686 {25s,35s} / veil of shadow 7068 {25s,35s} /
-- cleave 20691 7s / classcall {30s,35s} then {30s,35s} — all
-- non-triggered DoCastVictim (C++ default). EVENT_TAILLASH is never
-- scheduled in C++ JustEngagedWith (the line is commented out
-- upstream), so the tail lash case is dead code and omitted
-- C++-exact. ClassCall: random alive player in the instance (C++
-- SelectTarget Random 100 yd; shared arena) -> switch on C++ class
-- id: mage Talk(4)/self 23410, warrior Talk(5)/self 23397, druid
-- Talk(6)/cast 23398 on the target, priest Talk(7)/self 23401,
-- paladin Talk(8)/self 23418, shaman Talk(9)/self 23425, warlock
-- Talk(10)/self 23427, hunter Talk(11)/self 23436, rogue
-- Talk(12)/self 23414, death knight Talk(13)/self 49576; the re-arm
-- fires unconditionally (C++-exact). OnTargetDied: 1-in-5
-- (rand32() % 5 == 0, C++-exact) -> Talk(SAY_SLAY=2, victim).
-- OnDied: Talk(SAY_DEATH=3). Sub-20%: C++ UpdateAI checks
-- !Phase3 && HealthBelowPct(20) every tick — modeled on the
-- pre-damage hook as post-damage health strictly below 20% (moroes
-- convention), once, Talk(SAY_RAISE_SKELETONS=1); the bone-construct
-- respawn loop has no bearer. OnLeaveCombat(2)/OnReset(23): cancel
-- timers, clear state. Nil-target ticks cast nothing but keep the
-- schedule (jeklik convention).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no
-- instance-script model — boss admission via the luaBossAI shim
-- (BossAI::_Reset/JustEngagedWith, _JustDied, and the
-- DATA_NEFARIAN bookkeeping arms skipped); the gossip hello text
-- comes from the DB (menu 6045) and is approximated — the real text
-- lives in the DB, not in C++ (vaelastrasz convention); the C++
-- BeginEvent faction/stand-state/barrier/immune-to-PC/NPC-flag and
-- AttackStart arms have no bridges — the timers are armed from the
-- gossip select (the BeginEvent site) and combat begins on pull; no
-- summon model — the drakonid waves, the Nefarian summon, the
-- chromatic-chaos/vaelastrasz-spawn UBRS casts, and the
-- SummonedCreatureDies bone-construct transformation (dead
-- drakonids -> entry 14605, not-selectable, passive) never happen;
-- no movement model — the Nefarian summon's MovePoint-to-throne and
-- the MovementInform point-1 DoZoneInCombat/AttackStart arms are
-- skipped (nefarian fights from where it stands); no SetData
-- bearer and no path/gameobject bridges — the UBRS-only arms
-- (EVENT_PATH_2/EVENT_PATH_3 MovePath, EVENT_CHAOS_1/2 gyth-facing
-- 16337, EVENT_SUCCESS_1/2 portcullis gameobjects 164726/175186 +
-- DespawnOrUnsummon, FindNearestCreature/FindNearestGameObject)
-- are not modeled; no threat model — ResetThreatList skipped; the
-- canDespawn/DespawnTimer/FAIL encounter-state arm (SetBossState
-- + bone-construct grid despawn) is blocked on the instance-script
-- model; the mind-control event (30-35s) is commented out upstream
-- and never scheduled — omitted C++-exact.

local ENTRY_VICTOR_NEFARIUS = 10162
local ENTRY_NEFARIAN = 11583

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_GAMES_BEGIN = "Let the games begin!"

-- Victor Nefarius
local SAY_GAMESBEGIN_1 = 12
local SAY_GAMESBEGIN_2 = 13

local SPELL_SHADOWBOLT = 22677
local SPELL_SHADOWBOLT_VOLLEY = 22665
local SPELL_FEAR_NEFARIUS = 22678

-- Nefarian
local SAY_RANDOM = 0
local SAY_RAISE_SKELETONS = 1
local SAY_SLAY = 2
local SAY_DEATH = 3
local SAY_MAGE = 4
local SAY_WARRIOR = 5
local SAY_DRUID = 6
local SAY_PRIEST = 7
local SAY_PALADIN = 8
local SAY_SHAMAN = 9
local SAY_WARLOCK = 10
local SAY_HUNTER = 11
local SAY_ROGUE = 12
local SAY_DEATH_KNIGHT = 13

local SPELL_SHADOWFLAME = 22539
local SPELL_BELLOWINGROAR = 22686
local SPELL_VEILOFSHADOW = 7068
local SPELL_CLEAVE = 20691

local SPELL_MAGE = 23410
local SPELL_WARRIOR = 23397
local SPELL_DRUID = 23398
local SPELL_PRIEST = 23401
local SPELL_PALADIN = 23418
local SPELL_SHAMAN = 23425
local SPELL_WARLOCK = 23427
local SPELL_HUNTER = 23436
local SPELL_ROGUE = 23414
local SPELL_DEATH_KNIGHT = 49576

local CLASS_WARRIOR = 1
local CLASS_PALADIN = 2
local CLASS_HUNTER = 3
local CLASS_ROGUE = 4
local CLASS_PRIEST = 5
local CLASS_DEATH_KNIGHT = 6
local CLASS_SHAMAN = 7
local CLASS_MAGE = 8
local CLASS_WARLOCK = 9
local CLASS_DRUID = 11

local timers = {}
local nefariusState = {}
local nefarianState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomAlivePlayer(creature)
    local players = alivePlayersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- Victor Nefarius: C++ Initialize — SpawnedAdds = 0.
local function nefariusInit(guid)
    nefariusState[guid] = { spawnedAdds = 0, eventStarted = false }
end

-- C++ EVENT_SHADOW_BOLT: 50/50 shadowbolt volley on the victim or
-- shadowbolt on a random alive player; re-arm {3s,10s}.
local function onNefariusShadowBolt(creature, guid)
    if math.random(0, 1) == 0 then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SHADOWBOLT_VOLLEY)
        end
    else
        local target = randomAlivePlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_SHADOWBOLT)
        end
    end
    schedule(guid, "shadowbolt", math.random(3000, 10000), function()
        onNefariusShadowBolt(creature, guid)
    end)
end

-- C++ EVENT_FEAR: non-triggered 22678 on a random alive player;
-- re-arm {10s,20s}.
local function onNefariusFear(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_FEAR_NEFARIUS)
    end
    schedule(guid, "fear", math.random(10000, 20000), function()
        onNefariusFear(creature, guid)
    end)
end

-- C++ EVENT_SPAWN_ADD: two drakonids per wave, no summon bridge — only
-- the counting/re-arm cycle is kept; at 42 the C++ summons Nefarian
-- and drops Nefarius out of the fight, so cancel the timers.
local function onNefariusSpawnAdd(creature, guid)
    local st = nefariusState[guid]
    if not st then
        return
    end
    st.spawnedAdds = st.spawnedAdds + 2
    if st.spawnedAdds >= 42 then
        cancelTimers(guid)
        return
    end
    schedule(guid, "spawn_add", 4000, function()
        onNefariusSpawnAdd(creature, guid)
    end)
end

-- C++ BeginEvent (from OnGossipSelect): Talk(SAY_GAMESBEGIN_1) happens
-- in the select handler; the faction/flag/barrier/stand/immunity arms
-- have no bridge. Talk(SAY_GAMESBEGIN_2), then arm the fight.
local function beginEvent(creature, guid)
    local st = nefariusState[guid]
    if not st then
        nefariusInit(guid)
        st = nefariusState[guid]
    end
    cancelTimers(guid)
    st.spawnedAdds = 0
    st.eventStarted = true
    creature:Talk(SAY_GAMESBEGIN_2)
    schedule(guid, "shadowbolt", math.random(3000, 10000), function()
        onNefariusShadowBolt(creature, guid)
    end)
    schedule(guid, "fear", math.random(10000, 20000), function()
        onNefariusFear(creature, guid)
    end)
    schedule(guid, "spawn_add", 10000, function()
        onNefariusSpawnAdd(creature, guid)
    end)
end

local function onNefariusGossipHello(event, player, creature)
    if not nefariusState[creature:GetGUID()] then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_GAMES_BEGIN,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    player:GossipSendMenu(0, creature)
end

-- C++ OnGossipSelect: menu 6045, option 0 -> CloseGossipMenuFor,
-- Talk(SAY_GAMESBEGIN_1), BeginEvent(player).
local function onNefariusGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    player:GossipComplete()
    if sender == GOSSIP_SENDER_MAIN and action == GOSSIP_ACTION_INFO_DEF + 1 then
        creature:Talk(SAY_GAMESBEGIN_1)
        beginEvent(creature, creature:GetGUID())
    end
end

local function onNefariusReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nefariusInit(guid)
end

-- Nefarian: C++ EVENT_SHADOWFLAME — non-triggered DoCastVictim 12s.
local function onNefarianShadowflame(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWFLAME)
    end
    schedule(guid, "shadowflame", 12000, function()
        onNefarianShadowflame(creature, guid)
    end)
end

-- C++ EVENT_FEAR — bellowing roar 22686, {25s,35s}.
local function onNefarianFear(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BELLOWINGROAR)
    end
    schedule(guid, "fear", math.random(25000, 35000), function()
        onNefarianFear(creature, guid)
    end)
end

-- C++ EVENT_VEILOFSHADOW — 7068, {25s,35s}.
local function onNefarianVeil(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_VEILOFSHADOW)
    end
    schedule(guid, "veil", math.random(25000, 35000), function()
        onNefarianVeil(creature, guid)
    end)
end

-- C++ EVENT_CLEAVE — 20691, 7s.
local function onNefarianCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 7000, function()
        onNefarianCleave(creature, guid)
    end)
end

-- C++ EVENT_CLASSCALL: random alive player -> Talk + class spell;
-- druid (23398) is cast on the target, everything else non-triggered
-- self-cast (C++ DoCast(me, ...) default); re-arm {30s,35s}
-- unconditional (C++-exact).
local function onNefarianClassCall(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        local class = target:GetClass()
        if class == CLASS_MAGE then
            creature:Talk(SAY_MAGE)
            creature:CastSpell(creature, SPELL_MAGE)
        elseif class == CLASS_WARRIOR then
            creature:Talk(SAY_WARRIOR)
            creature:CastSpell(creature, SPELL_WARRIOR)
        elseif class == CLASS_DRUID then
            creature:Talk(SAY_DRUID)
            creature:CastSpell(target, SPELL_DRUID)
        elseif class == CLASS_PRIEST then
            creature:Talk(SAY_PRIEST)
            creature:CastSpell(creature, SPELL_PRIEST)
        elseif class == CLASS_PALADIN then
            creature:Talk(SAY_PALADIN)
            creature:CastSpell(creature, SPELL_PALADIN)
        elseif class == CLASS_SHAMAN then
            creature:Talk(SAY_SHAMAN)
            creature:CastSpell(creature, SPELL_SHAMAN)
        elseif class == CLASS_WARLOCK then
            creature:Talk(SAY_WARLOCK)
            creature:CastSpell(creature, SPELL_WARLOCK)
        elseif class == CLASS_HUNTER then
            creature:Talk(SAY_HUNTER)
            creature:CastSpell(creature, SPELL_HUNTER)
        elseif class == CLASS_ROGUE then
            creature:Talk(SAY_ROGUE)
            creature:CastSpell(creature, SPELL_ROGUE)
        elseif class == CLASS_DEATH_KNIGHT then
            creature:Talk(SAY_DEATH_KNIGHT)
            creature:CastSpell(creature, SPELL_DEATH_KNIGHT)
        end
    end
    schedule(guid, "classcall", math.random(30000, 35000), function()
        onNefarianClassCall(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_RANDOM), arm the fight. The
-- EVENT_TAILLASH schedule line is commented out upstream — the tail
-- lash 23364 case is dead code and stays omitted C++-exact.
local function nefarianEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nefarianState[guid] = { phase3 = false }
    creature:Talk(SAY_RANDOM)
    schedule(guid, "shadowflame", 12000, function()
        onNefarianShadowflame(creature, guid)
    end)
    schedule(guid, "fear", math.random(25000, 35000), function()
        onNefarianFear(creature, guid)
    end)
    schedule(guid, "veil", math.random(25000, 35000), function()
        onNefarianVeil(creature, guid)
    end)
    schedule(guid, "cleave", 7000, function()
        onNefarianCleave(creature, guid)
    end)
    schedule(guid, "classcall", math.random(30000, 35000), function()
        onNefarianClassCall(creature, guid)
    end)
end

-- C++ KilledUnit: rand32() % 5 == 0 -> Talk(SAY_SLAY, victim).
local function nefarianTargetDied(event, creature, victim)
    if math.random(0, 4) == 0 then
        creature:Talk(SAY_SLAY, victim)
    end
end

-- C++ UpdateAI sub-20% phase 3: !Phase3 && HealthBelowPct(20). The
-- damage hook fires pre-application, so the post-damage health
-- decides (moroes convention); strictly below 20 like C++; once.
-- The bone-construct respawn loop has no bearer.
local function nefarianDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = nefarianState[guid]
    if st == nil or st.phase3 then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 20 then
        st.phase3 = true
        creature:Talk(SAY_RAISE_SKELETONS)
    end
end

local function nefarianClear(guid)
    cancelTimers(guid)
    nefarianState[guid] = nil
end

local function nefarianLeaveCombat(event, creature)
    nefarianClear(creature:GetGUID())
end

-- C++ JustDied: Talk(SAY_DEATH).
local function nefarianDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    nefarianClear(creature:GetGUID())
end

local function nefarianReset(event, creature)
    nefarianClear(creature:GetGUID())
end

RegisterCreatureGossipEvent(ENTRY_VICTOR_NEFARIUS, 1, onNefariusGossipHello)
RegisterCreatureGossipEvent(ENTRY_VICTOR_NEFARIUS, 2, onNefariusGossipSelect)
RegisterCreatureEvent(ENTRY_VICTOR_NEFARIUS, 2, onNefariusReset)
RegisterCreatureEvent(ENTRY_VICTOR_NEFARIUS, 4, onNefariusReset)
RegisterCreatureEvent(ENTRY_VICTOR_NEFARIUS, 23, onNefariusReset)

RegisterCreatureEvent(ENTRY_NEFARIAN, 1, nefarianEnterCombat)
RegisterCreatureEvent(ENTRY_NEFARIAN, 2, nefarianLeaveCombat)
RegisterCreatureEvent(ENTRY_NEFARIAN, 3, nefarianTargetDied)
RegisterCreatureEvent(ENTRY_NEFARIAN, 4, nefarianDied)
RegisterCreatureEvent(ENTRY_NEFARIAN, 9, nefarianDamageTaken)
RegisterCreatureEvent(ENTRY_NEFARIAN, 23, nefarianReset)
