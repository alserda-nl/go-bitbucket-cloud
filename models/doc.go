// Package models holds the data types of Bitbucket Cloud's answers, one type per object, named
// after the API's schema. A field Bitbucket sends that its specification leaves out is here too.
//
// Another object embeds a shorter form of some of them, such as a project's workspace: the fields
// that form leaves out are zero there.
package models
