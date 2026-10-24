-- Jan'alai (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_janalai.cpp
-- (boss_janalaiAI + npc_janalai_firebombAI + npc_janalai_hatcherAI +
-- npc_janalai_hatchlingAI + npc_janalai_eggAI); zulaman.h:30
-- (BOSS_JANALAI = 2), zulaman.h:47 (NPC_JANALAI = 23578).
-- Creature entries: 23578 Jan'alai (C++ ScriptName "boss_janalai" per
-- AddSC_boss_janalai); 23920 fire bomb ("npc_janalai_firebomb");
-- 23818 Amani hatcher ("npc_janalai_hatcher"); 23598 dragonhawk hatchling
-- ("npc_janalai_hatchling"); 23817 egg ("npc_janalai_egg").
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 14 OnSpellHit, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Fight shape: flame breath 43140 on a random player at 8s, repeat 8s
-- (C++ motion-master Clear still has no bridge and the DamageDealt
-- 30-degree arc gate has no bridge, so the isFlameBreathing UpdateAI
-- gate never runs; StopMoving IS bridged and IS ported in onFlameBreath
-- below — the old "StopMoving has no bridge" claim was stale); the bomb phase
-- IS ported (2026-10-24 — the old "no summon/teleport bridge" claim was
-- stale: creature:SummonCreature, creature:Teleport, creature:AttackStop,
-- creature:StopMoving and player:TeleportNear are all bridged): BombTimer
-- 30s -> Talk(SAY_FIRE_BOMBS=1) + AttackStop + Teleport to the platform
-- center + StopMoving + non-triggered self-cast FIRE_BOMB_CHANNEL 42621 +
-- FireWall() (10 wall bombs at FireWallCoords, TEMPSUMMON_TIMED_DESPAWN
-- type 2 / 15s, each triggered self-cast FIRE_WALL 43113) + SpawnBombs()
-- (40 bombs at boss-relative irand(-22,22)/irand(-25,25) offsets, same
-- type/despawn, tracked in st.bombs) + alive players teleported near the
-- center (C++ DoTeleportPlayer) + the flamebreath/hatcher timers cancelled
-- (C++ pauses them while isBombing) + 100ms bomb-sequence pump: per bomb
-- RemoveFlag(NOT_SELECTABLE) + boss DoCast(FIRE_BOMB_THROW 42628,
-- triggered) + SetFlag(NOT_SELECTABLE) (the registered firebomb SpellHit
-- arm answers with triggered 42629), re-arm 100ms, 5000ms after the 40th
-- -> Boom(): every tracked bomb takes triggered FIRE_BOMB_DAMAGE 42630 +
-- RemoveAllAuras, then BombTimer = urand(20000,40000), RemoveAura(42621),
-- flamebreath/hatcher re-armed at full delays and enrage re-armed at
-- 290000 (the C++ 10s EnrageTimer shave rides the full-delay re-arm —
-- the ~9s bombing pause has no remaining-time bridge, documented
-- micro-deviation); melee is suppressed while bombing via SetMeleeAttack
-- (C++ UpdateAI returns before DoMeleeAttackIfReady when isBombing).
-- HatcherTimer 10s/90s IS ported: HatchAllEggs(0) is a pure egg-existence
-- probe, modeled with creature:GetCreaturesInGrid(ENTRY_EGG, 100) — eggs
-- present -> Talk(SAY_SUMMON_HATCHER=2) + two 23818 summons at the
-- hatcherway spawn points (TEMPSUMMON_CORPSE_TIMED_DESPAWN type 5 / 10s)
-- + 90s re-arm, none -> noeggs latches the arm off (C++-exact); the
-- hatchers' own waypoint/egg-hatching AI stays unregistered (movement-
-- driven body, fiendish-portal precedent), so summoned hatchers idle.
-- a 5min one-shot (EnrageTimer = 5*60*1000) whose first fire triggered-
-- casts enrage 44779, sets enraged, and re-arms 300s, and whose later
-- fires Talk(SAY_BERSERK=4) + triggered berserk 45078 + re-arm 300s
-- (C++-exact); the sub-25% early trigger (C++: EnrageTimer = 0 when
-- HealthBelowPct(25)) runs in the event-9 hook on projected
-- post-damage health (nefarian rule), canceling the pending timer
-- and firing the same handler. Kills: Talk SAY_SLAY (C++ KilledUnit has no player gate);
-- death: Talk SAY_DEATH.
-- Deviations from C++: the bomb-phase MotionMaster Clear is cosmetic and
-- skipped (the teleport lands anyway); the sub-25% enrage event-9 hook can
-- fire mid-bombing while C++ defers it until the sequence ends (~9s
-- ordering delta); the hatcher AI is not registered at all — its whole
-- body is MovementInform waypoints, MoveInLineOfSight/AttackStart gates,
-- and instance-state checks (fiendish-portal precedent: no Lua-modelable
-- hooks fire for it), so the two summoned hatchers idle; the fire bomb
-- 23920 SpellHit arm (42628 -> triggered self-cast 42629) answers the
-- sequence throws; the hatchling 23598 flame-buffet AI and the egg 23817
-- SpellHit arm (42471 -> self-cast 42493) are registered so hatched eggs
-- spawn hatchlings; the Reset egg display-id restore (HatchAllEggs(1),
-- display 10056) is ported in janalaiReset via GetCreaturesInGrid;
-- no instance-script model — admission only via the luaBossAI shim
-- (GetZulAmanAI/BossAI SetBossState arms skipped, the hatchling's
-- IN_PROGRESS DisappearAndDie arm skipped); the pre-fight trash
-- gauntlet's SAY_EVENT_STRANGERS/FRIENDS lines belong to zulaman.cpp,
-- not this file's AI; the sub-25% enrage arm is a C++ UpdateAI
-- HealthBelowPct, so the event-9 port tests projected post-damage health
-- (postDamage > 0 and postDamage < floor threshold — nefarian rule)
-- rather than pre-damage health.

local ENTRY_JANALAI = 23578
local ENTRY_FIRE_BOMB = 23920
local ENTRY_HATCHLING = 23598
local ENTRY_EGG = 23817
local ENTRY_HATCHER = 23818

local SAY_AGGRO = 0
local SAY_FIRE_BOMBS = 1
local SAY_SUMMON_HATCHER = 2
local SAY_ALL_EGGS = 3
local SAY_BERSERK = 4
local SAY_SLAY = 5
local SAY_DEATH = 6

local SPELL_FLAME_BREATH = 43140
local SPELL_ENRAGE = 44779
local SPELL_BERSERK = 45078
local SPELL_HATCH_ALL = 43144

-- C++ JanalainPos[0] (boss_janalai.cpp:84-87): the platform center both
-- the bomb phase and the all-eggs arm teleport Jan'alai to.
local JANALAI_CENTER_X = -33.93
local JANALAI_CENTER_Y = 1149.27
local JANALAI_CENTER_Z = 19

local SPELL_FIRE_BOMB_THROW = 42628
local SPELL_FIRE_BOMB_DUMMY = 42629

local SPELL_FLAMEBUFFET = 43299

local SPELL_HATCH_EGG = 42471
local SPELL_SUMMON_HATCHLING = 42493

local SPELL_FIRE_WALL = 43113
local SPELL_FIRE_BOMB_CHANNEL = 42621
local SPELL_FIRE_BOMB_DAMAGE = 42630

-- SetFlag/RemoveFlag field + flag values (opera precedent):
-- UNIT_FIELD_FLAGS = OBJECT_END(0x6)+0x35 = 59 (UpdateFields.h:117);
-- UNIT_FLAG_NOT_SELECTABLE = 0x02000000 (UnitDefines.h:149).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- C++ FireWallCoords (boss_janalai.cpp:89-95): the four fire-wall lines.
local FIRE_WALL_COORDS = {
    {-10.13, 1149.27, 19, 3.1415},
    {-33.93, 1123.90, 19, 0.5 * 3.1415},
    {-54.80, 1150.08, 19, 0},
    {-33.93, 1175.68, 19, 1.5 * 3.1415},
}

-- C++ hatcherway (boss_janalai.cpp:97-115): the two hatcher spawn points
-- (hatcherway[0][0] and hatcherway[1][0]); the waypoint legs are movement-
-- bridged-out, documented below.
local HATCHER_SPAWNS = {
    {-87.46, 1170.09, 6},
    {-86.57, 1132.85, 6},
}

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function janalaiState(guid)
    local st = state[guid]
    if not st then
        st = { enraged = false, noeggs = false, bombing = false,
               bombCount = 0, bombs = {}, wallBombs = {} }
        state[guid] = st
    end
    return st
end

local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ flame breath: random target (Random,0 includes the tank), 8s
-- repeat; no triggered flag on the cast.
local function onFlameBreath(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        -- C++ boss_janalai.cpp:417-420: me->AttackStop() + DoCast
        -- before the flame breath cast; me->StopMoving() stops the
        -- breath-caster mid-stride (motion Master Clear stays unbridged).
        creature:AttackStop()
        creature:CastSpell(target, SPELL_FLAME_BREATH)
        creature:StopMoving()
    end
    schedule(timers, guid, "flamebreath", 8000, function()
        onFlameBreath(creature, guid)
    end)
end

-- C++ EnrageTimer arm: first fire -> triggered self-cast 44779, enraged,
-- re-arm 300s; later fires -> Talk SAY_BERSERK, triggered 45078, re-arm
-- 300s (C++-exact).
local function onEnrage(creature, guid)
    local st = janalaiState(guid)
    if not st.enraged then
        creature:CastSpell(creature, SPELL_ENRAGE, true)
        st.enraged = true
    else
        creature:Talk(SAY_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK, true)
    end
    schedule(timers, guid, "enrage", 300000, function()
        onEnrage(creature, guid)
    end)
end

-- Despawn any tracked bombs (BossAI evade/death despawns summons; the Lua
-- tracks them per-guid, vancleef blackguard idiom).
local function despawnTrackedBombs(guid)
    local st = state[guid]
    if not st then
        return
    end
    for _, list in ipairs({ st.bombs, st.wallBombs }) do
        for _, bomb in ipairs(list) do
            if bomb then
                bomb:Despawn()
            end
        end
    end
end

-- Forward declaration: onBombPhase, onBombSequence and onHatcher call
-- each other across definition order (the tharon_ja/gal_darah idiom — a
-- plain assignment, never a second `local function`, or the closures bind
-- a nil slot).
local onBombPhase, onHatcher

-- C++ HandleBombSequence (boss_janalai.cpp:275-296): FireBombGUIDs holds
-- the 40 SpawnBombs summons in order; each tick the next bomb loses
-- NOT_SELECTABLE, takes the boss's triggered FIRE_BOMB_THROW 42628 (the
-- registered firebomb SpellHit arm answers with triggered 42629), and
-- regains NOT_SELECTABLE. Re-arm is 100ms, 5000ms after the 40th; then
-- Boom() (boss_janalai.cpp:258-273): every fire bomb in 100 yd — exactly
-- the 50 tracked summons — takes triggered FIRE_BOMB_DAMAGE 42630 and
-- loses all auras. Afterwards BombTimer = urand(20000,40000), the channel
-- aura is removed, and the paused timers resume (C++-exact re-arms; the
-- enrage 10s shave rides the full-delay re-arm, documented above).
local function onBombSequence(creature, guid)
    local st = janalaiState(guid)
    if st.bombCount < 40 then
        local bomb = st.bombs[st.bombCount + 1]
        if bomb then
            bomb:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:CastSpell(bomb, SPELL_FIRE_BOMB_THROW, true)
            bomb:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        end
        st.bombCount = st.bombCount + 1
        schedule(timers, guid, "bombseq",
            st.bombCount == 40 and 5000 or 100, function()
                onBombSequence(creature, guid)
            end)
        return
    end
    for _, bomb in ipairs(st.bombs) do
        bomb:CastSpell(bomb, SPELL_FIRE_BOMB_DAMAGE, true)
        bomb:RemoveAllAuras()
    end
    for _, bomb in ipairs(st.wallBombs) do
        bomb:CastSpell(bomb, SPELL_FIRE_BOMB_DAMAGE, true)
        bomb:RemoveAllAuras()
    end
    st.bombing = false
    st.bombCount = 0
    st.bombs = {}
    st.wallBombs = {}
    creature:RemoveAura(SPELL_FIRE_BOMB_CHANNEL)
    creature:SetMeleeAttack(true)
    schedule(timers, guid, "bomb", math.random(20000, 40000), function()
        onBombPhase(creature, guid)
    end)
    schedule(timers, guid, "flamebreath", 8000, function()
        onFlameBreath(creature, guid)
    end)
    schedule(timers, guid, "hatcher", 90000, function()
        onHatcher(creature, guid)
    end)
    schedule(timers, guid, "enrage", 290000, function()
        onEnrage(creature, guid)
    end)
end

-- C++ BombTimer arm (boss_janalai.cpp:348-380): Talk(SAY_FIRE_BOMBS),
-- AttackStop, MotionMaster Clear (cosmetic, no bridge), DoTeleportTo the
-- platform center, StopMoving, non-triggered self-cast FIRE_BOMB_CHANNEL
-- 42621, FireWall() (10 bombs on the four wall lines), SpawnBombs() (40
-- bombs at boss-relative irand(-22,22)/irand(-25,25) offsets — C++
-- DoSpawnCreature adds offsets to the boss position,
-- ScriptedCreature.cpp:286-289), every alive player teleported near the
-- center (C++ DoTeleportPlayer; Zul'Aman is always a dungeon so the
-- IsDungeon early-out never triggers), then isBombing with the 100ms
-- sequence pump. Flamebreath/hatcher timers are cancelled here because
-- C++ pauses them while isBombing (UpdateAI early-return); the enrage
-- timer is re-armed at bomb end (see onBombSequence).
onBombPhase = function(creature, guid)
    local st = janalaiState(guid)
    creature:Talk(SAY_FIRE_BOMBS)
    creature:AttackStop()
    creature:Teleport(JANALAI_CENTER_X, JANALAI_CENTER_Y, JANALAI_CENTER_Z)
    creature:StopMoving()
    creature:SetMeleeAttack(false)
    creature:CastSpell(creature, SPELL_FIRE_BOMB_CHANNEL)
    st.bombs = {}
    st.wallBombs = {}
    for i = 1, 4 do
        local wc = FIRE_WALL_COORDS[i]
        local wallNum = (i == 1 or i == 3) and 3 or 2
        for j = 1, wallNum do
            local x, y
            if wallNum == 3 then
                x, y = wc[1], wc[2] + 5 * (j - 2)
            else
                x, y = wc[1] - 2 + 4 * (j - 1), wc[2]
            end
            local bomb = creature:SummonCreature(ENTRY_FIRE_BOMB,
                x, y, wc[3], wc[4], 2, 15000)
            if bomb then
                bomb:CastSpell(bomb, SPELL_FIRE_WALL, true)
                st.wallBombs[#st.wallBombs + 1] = bomb
            end
        end
    end
    local bx, by, bz = creature:GetX(), creature:GetY(), creature:GetZ()
    for _ = 1, 40 do
        local bomb = creature:SummonCreature(ENTRY_FIRE_BOMB,
            bx + math.random(-22, 22), by + math.random(-25, 25), bz,
            0, 2, 15000)
        if bomb then
            st.bombs[#st.bombs + 1] = bomb
        end
    end
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            p:TeleportNear(JANALAI_CENTER_X - 5 + math.random(0, 9),
                JANALAI_CENTER_Y - 5 + math.random(0, 9),
                JANALAI_CENTER_Z, 0)
        end
    end
    local per = timers[guid]
    if per then
        if per["flamebreath"] then
            RemoveEventById(per["flamebreath"])
            per["flamebreath"] = nil
        end
        if per["hatcher"] then
            RemoveEventById(per["hatcher"])
            per["hatcher"] = nil
        end
    end
    st.bombing = true
    st.bombCount = 0
    schedule(timers, guid, "bombseq", 100, function()
        onBombSequence(creature, guid)
    end)
end

-- C++ HatcherTimer arm (boss_janalai.cpp:398-408): HatchAllEggs(0) is a
-- pure existence probe (action 0 touches no egg) over NPC_EGG within
-- 100 yd, modeled with creature:GetCreaturesInGrid. Eggs present ->
-- Talk(SAY_SUMMON_HATCHER) + two 23818 summons at the hatcherway spawn
-- points (TEMPSUMMON_CORPSE_TIMED_DESPAWN, Go type 5, 10s) + 90s re-arm;
-- none -> noeggs latches the arm off, C++-exact.
onHatcher = function(creature, guid)
    local st = janalaiState(guid)
    if #creature:GetCreaturesInGrid(ENTRY_EGG, 100) > 0 then
        creature:Talk(SAY_SUMMON_HATCHER)
        for _, pos in ipairs(HATCHER_SPAWNS) do
            creature:SummonCreature(ENTRY_HATCHER,
                pos[1], pos[2], pos[3], 0, 5, 10000)
        end
        schedule(timers, guid, "hatcher", 90000, function()
            onHatcher(creature, guid)
        end)
    else
        st.noeggs = true
    end
end

local function janalaiResetState(guid)
    despawnTrackedBombs(guid)
    cancelTimers(timers, guid)
    state[guid] = { enraged = false, noeggs = false, bombing = false,
                    bombCount = 0, bombs = {}, wallBombs = {} }
end

local function janalaiEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    janalaiResetState(guid)
    -- C++ JustEngagedWith (boss_janalai.cpp:181-193): Talk + the Initialize
    -- timer set — FireBreathTimer 8s, BombTimer 30s, HatcherTimer 10s,
    -- EnrageTimer 5min (the old "no summon/teleport bridge" claim was
    -- stale; the bomb and hatcher arms are ported above).
    creature:Talk(SAY_AGGRO)
    schedule(timers, guid, "flamebreath", 8000, function()
        onFlameBreath(creature, guid)
    end)
    schedule(timers, guid, "bomb", 30000, function()
        onBombPhase(creature, guid)
    end)
    schedule(timers, guid, "hatcher", 10000, function()
        onHatcher(creature, guid)
    end)
    schedule(timers, guid, "enrage", 300000, function()
        onEnrage(creature, guid)
    end)
end

-- C++ UpdateAI: if !enraged and HealthBelowPct(25) -> EnrageTimer = 0
-- (fires immediately). Modeled in the event-9 hook on projected
-- post-damage health (nefarian rule: postDamage = max(health -
-- damage, 0); fires when 0 < postDamage < floor(max*25/100)),
-- canceling the pending 5min timer. C++ is UpdateAI post-damage,
-- so it never fires on the killing blow; the postDamage > 0 guard
-- keeps the port from enraging while dying.
local function janalaiDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = janalaiState(guid)
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = math.max(health - damage, 0)
    -- C++ UpdateAI !noeggs + HealthBelowPct(35) arm (boss_janalai.cpp:
    -- 381-397): once per fight — Talk(SAY_ALL_EGGS), teleport to the
    -- platform center, non-triggered self-cast HATCH_ALL, then
    -- HatchAllEggs(2): every egg within 100 yd whose display is not
    -- 11686 self-casts HATCH_EGG (the registered egg SpellHit arm then
    -- summons the hatchling). Modeled on projected post-damage health
    -- (nefarian rule), independent of the enrage latch below.
    if not st.noeggs and postDamage > 0
            and postDamage < math.floor(maxHealth * 35 / 100) then
        st.noeggs = true
        creature:Talk(SAY_ALL_EGGS)
        creature:AttackStop()
        creature:Teleport(JANALAI_CENTER_X, JANALAI_CENTER_Y, JANALAI_CENTER_Z)
        creature:StopMoving()
        creature:CastSpell(creature, SPELL_HATCH_ALL)
        for _, egg in ipairs(creature:GetCreaturesInGrid(ENTRY_EGG, 100)) do
            if egg:GetDisplayId() ~= 11686 then
                egg:CastSpell(egg, SPELL_HATCH_EGG)
            end
        end
    end
    if st.enraged then
        return
    end
    if postDamage > 0 and postDamage < math.floor(maxHealth * 25 / 100) then
        local per = timers[guid]
        if per and per["enrage"] then
            RemoveEventById(per["enrage"])
            per["enrage"] = nil
        end
        onEnrage(creature, guid)
    end
end

local function janalaiLeaveCombat(event, creature)
    janalaiResetState(creature:GetGUID())
end

local function janalaiTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

local function janalaiDied(event, creature, killer)
    janalaiResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function janalaiReset(event, creature)
    janalaiResetState(creature:GetGUID())
    -- C++ boss_janalai::Reset (boss_janalai.cpp:162-169): Initialize()
    -- -> position-gated MovePoint (no movement bridge) ->
    -- me->SetDisableGravity(true).
    creature:SetDisableGravity(true)
    -- C++ HatchAllEggs(1) (:166, :234-256): restore display 10056 on
    -- every egg within 100 yd.
    for _, egg in ipairs(creature:GetCreaturesInGrid(ENTRY_EGG, 100)) do
        egg:SetDisplayId(10056)
    end
end

RegisterCreatureEvent(ENTRY_JANALAI, 1, janalaiEnterCombat)
RegisterCreatureEvent(ENTRY_JANALAI, 2, janalaiLeaveCombat)
RegisterCreatureEvent(ENTRY_JANALAI, 3, janalaiTargetDied)
RegisterCreatureEvent(ENTRY_JANALAI, 4, janalaiDied)
RegisterCreatureEvent(ENTRY_JANALAI, 9, janalaiDamageTaken)
RegisterCreatureEvent(ENTRY_JANALAI, 23, janalaiReset)

-- npc_janalai_firebomb (23920, C++ ScriptName for the entry): SpellHit on
-- FIRE_BOMB_THROW 42628 -> triggered self-cast FIRE_BOMB_DUMMY 42629.
-- Fire bombs never spawn in this build (no summon model), but the AI is
-- registered so any that exist react correctly.
local function firebombSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_FIRE_BOMB_THROW then
        creature:CastSpell(creature, SPELL_FIRE_BOMB_DUMMY, true)
    end
end

RegisterCreatureEvent(ENTRY_FIRE_BOMB, 14, firebombSpellHit)
RegisterCreatureEvent(ENTRY_FIRE_BOMB, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_janalai.cpp:459: empty UpdateAI in C++ (never melees)
end)

-- C++ npc_janalai_firebombAI::MoveInLineOfSight (boss_janalai.cpp:457) is an
-- EMPTY override — it suppresses the default ScriptedAI::MoveInLineOfSight
-- aggro engage (the fire bomb is a thrown object, never attacks). Go's
-- fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models it C++-exact (the cyanigosa precedent).
local function firebombMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(ENTRY_FIRE_BOMB, 27, firebombMoveInLOS)

-- npc_janalai_hatchling (23598, C++ ScriptName "npc_janalai_hatchling"):
-- flame buffet 43299 on the victim, first at 7s then every 10s; melee is
-- engine-driven. The MovePoint spawn walk and the instance-state
-- DisappearAndDie arm have no bridge; Reset's SetDisableGravity ports
-- via the creature:SetDisableGravity bridge (hatchlingReset).
-- Hatchlings never
-- spawn in this build (no summon model), but the AI is registered so any
-- that exist fight correctly.
local function onBuffet(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBUFFET)
    end
    schedule(timers, guid, "buffet", 10000, function()
        onBuffet(creature, guid)
    end)
end

local function hatchlingEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    schedule(timers, guid, "buffet", 7000, function()
        onBuffet(creature, guid)
    end)
end

local function hatchlingLeaveCombat(event, creature)
    cancelTimers(timers, creature:GetGUID())
end

local function hatchlingReset(event, creature)
    cancelTimers(timers, creature:GetGUID())
    -- C++ npc_janalai_hatchlingAI::Reset (boss_janalai.cpp:618-627):
    -- Initialize() -> position-gated MovePoint (no movement bridge) ->
    -- me->SetDisableGravity(true) (:626).
    creature:SetDisableGravity(true)
end

RegisterCreatureEvent(ENTRY_HATCHLING, 1, hatchlingEnterCombat)
RegisterCreatureEvent(ENTRY_HATCHLING, 2, hatchlingLeaveCombat)
RegisterCreatureEvent(ENTRY_HATCHLING, 23, hatchlingReset)

-- npc_janalai_egg (23817, C++ ScriptName "npc_janalai_egg"): SpellHit on
-- HATCH_EGG 42471 -> self-cast SUMMON_HATCHLING 42493. Eggs never spawn in
-- this build (no summon model), but the AI is registered so any that
-- exist react correctly.
local function eggSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_HATCH_EGG then
        creature:CastSpell(creature, SPELL_SUMMON_HATCHLING)
    end
end

RegisterCreatureEvent(ENTRY_EGG, 14, eggSpellHit)
RegisterCreatureEvent(ENTRY_EGG, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_janalai.cpp:671: empty UpdateAI in C++ (never melees)
end)
