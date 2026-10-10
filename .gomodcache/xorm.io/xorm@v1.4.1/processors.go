// Copyright 2015 The Xorm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xorm

// BeforeInsertProcessor executed before an object is initially persisted to the database
type BeforeInsertProcessor interface {
	BeforeInsert()
}

// BeforeUpdateProcessor executed before an object is updated
type BeforeUpdateProcessor interface {
	BeforeUpdate()
}

// BeforeDeleteProcessor executed before an object is deleted
type BeforeDeleteProcessor interface {
	BeforeDelete()
}

// BeforeSetProcessor executed before data set to the struct fields
type BeforeSetProcessor interface {
	BeforeSet(string, Cell)
}

// AfterSetProcessor executed after data set to the struct fields
type AfterSetProcessor interface {
	AfterSet(string, Cell)
}

// AfterInsertProcessor executed after an object is persisted to the database
type AfterInsertProcessor interface {
	AfterInsert()
}

// AfterUpdateProcessor executed after an object has been updated
type AfterUpdateProcessor interface {
	AfterUpdate()
}

// AfterDeleteProcessor executed after an object has been deleted
type AfterDeleteProcessor interface {
	AfterDelete()
}

// AfterLoadProcessor executed after an ojbect has been loaded from database
type AfterLoadProcessor interface {
	AfterLoad()
}

// AfterLoadSessionProcessor executed after an ojbect has been loaded from database with session parameter
type AfterLoadSessionProcessor interface {
	AfterLoad(*Session)
}

type executedProcessorFunc func(*Session, any) error

type executedProcessor struct {
	fun     executedProcessorFunc
	session *Session
	bean    any
}

func (executor *executedProcessor) execute() error {
	return executor.fun(executor.session, executor.bean)
}

func (session *Session) executeProcessors() error {
	processors := session.afterProcessors
	session.afterProcessors = make([]executedProcessor, 0)
	session.resetStatement()
	for _, processor := range processors {
		if err := processor.execute(); err != nil {
			return err
		}
	}
	return nil
}

func cleanupProcessorsClosures(slices *[]func(any)) {
	if len(*slices) > 0 {
		*slices = make([]func(any), 0)
	}
}

func executeBeforeClosures(session *Session, bean any) {
	// handle before delete processors
	for _, closure := range session.beforeClosures {
		closure(bean)
	}
	cleanupProcessorsClosures(&session.beforeClosures)
}

func executeBeforeSet(bean any, fields []string, scanResults []any) {
	if b, hasBeforeSet := bean.(BeforeSetProcessor); hasBeforeSet {
		for ii, key := range fields {
			b.BeforeSet(key, Cell(scanResults[ii].(*any)))
		}
	}
}

func executeAfterSet(bean any, fields []string, scanResults []any) {
	if b, hasAfterSet := bean.(AfterSetProcessor); hasAfterSet {
		for ii, key := range fields {
			b.AfterSet(key, Cell(scanResults[ii].(*any)))
		}
	}
}

func buildAfterProcessors(session *Session, bean any) {
	// handle afterClosures
	for _, closure := range session.afterClosures {
		afterClosure := closure
		session.afterProcessors = append(session.afterProcessors, executedProcessor{
			fun: func(sess *Session, bean any) error {
				afterClosure(bean)
				return nil
			},
			session: session,
			bean:    bean,
		})
	}

	if a, has := bean.(AfterLoadProcessor); has {
		session.afterProcessors = append(session.afterProcessors, executedProcessor{
			fun: func(sess *Session, bean any) error {
				a.AfterLoad()
				return nil
			},
			session: session,
			bean:    bean,
		})
	}

	if a, has := bean.(AfterLoadSessionProcessor); has {
		session.afterProcessors = append(session.afterProcessors, executedProcessor{
			fun: func(sess *Session, bean any) error {
				a.AfterLoad(sess)
				return nil
			},
			session: session,
			bean:    bean,
		})
	}
}
